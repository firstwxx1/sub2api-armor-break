package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type armorBreakFakeRepo struct {
	values map[string]string
}

func (r *armorBreakFakeRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: v}, nil
}

func (r *armorBreakFakeRepo) GetValue(_ context.Context, key string) (string, error) {
	s, err := r.Get(context.Background(), key)
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

func (r *armorBreakFakeRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *armorBreakFakeRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *armorBreakFakeRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *armorBreakFakeRepo) GetAll(_ context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *armorBreakFakeRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

// newArmorBreakTestRig 起一条带破甲中间件的 gin 链，终态 handler 把读到的 body
// 原样回写，断言用。
func newArmorBreakTestRig(t *testing.T, svc *service.ArmorBreakService, path string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST(path, ArmorBreak(svc), func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.Data(http.StatusOK, "application/json", body)
	})
	return r
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) string {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	return w.Body.String()
}

func TestArmorBreakMiddlewareDisabledPassthrough(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{}}
	svc := service.NewArmorBreakService(repo, t.TempDir())
	r := newArmorBreakTestRig(t, svc, "/v1/messages")

	raw := `{"system":"ORIG","messages":[]}`
	require.JSONEq(t, raw, postJSON(t, r, "/v1/messages", raw))
}

func TestArmorBreakMiddlewareRewritesAnthropicSystem(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{
		service.SettingKeyArmorBreakEnabled: "true",
		service.SettingKeyArmorBreakPersona: "R.txt",
	}}
	dir := t.TempDir()
	svc := service.NewArmorBreakService(repo, dir)
	_, err := svc.Store().Put("R.txt", "人格原文")
	require.NoError(t, err)

	r := newArmorBreakTestRig(t, svc, "/v1/messages")
	got := postJSON(t, r, "/v1/messages", `{"system":"ORIG","messages":[{"role":"user","content":"hi"}]}`)
	require.Contains(t, got, `"system":"人格原文"`)
	require.Contains(t, got, `"role":"user"`)
}

func TestArmorBreakMiddlewareRewritesOpenAIChat(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{
		service.SettingKeyArmorBreakEnabled: "true",
		service.SettingKeyArmorBreakPersona: "R.txt",
	}}
	svc := service.NewArmorBreakService(repo, t.TempDir())
	_, err := svc.Store().Put("R.txt", "人格原文")
	require.NoError(t, err)

	r := newArmorBreakTestRig(t, svc, "/v1/chat/completions")
	got := postJSON(t, r, "/v1/chat/completions", `{"model":"gpt","messages":[{"role":"user","content":"hi"}]}`)
	require.Contains(t, got, `"role":"system"`)
	require.Contains(t, got, `"content":"人格原文"`)
}

func TestArmorBreakMiddlewareFailOpenOnMissingPersona(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{
		service.SettingKeyArmorBreakEnabled: "true",
		service.SettingKeyArmorBreakPersona: "ghost.txt",
	}}
	svc := service.NewArmorBreakService(repo, t.TempDir())

	r := newArmorBreakTestRig(t, svc, "/v1/messages")
	raw := `{"system":"ORIG","messages":[]}`
	require.JSONEq(t, raw, postJSON(t, r, "/v1/messages", raw), "missing persona must passthrough")
}

func TestArmorBreakMiddlewareSkipsUntargetedPaths(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{
		service.SettingKeyArmorBreakEnabled: "true",
		service.SettingKeyArmorBreakPersona: "R.txt",
	}}
	svc := service.NewArmorBreakService(repo, t.TempDir())
	_, err := svc.Store().Put("R.txt", "人格原文")
	require.NoError(t, err)

	// count_tokens 不改写
	r := newArmorBreakTestRig(t, svc, "/v1/messages/count_tokens")
	raw := `{"system":"ORIG","messages":[]}`
	require.JSONEq(t, raw, postJSON(t, r, "/v1/messages/count_tokens", raw))
}

func TestArmorBreakMiddlewareNilService(t *testing.T) {
	r := newArmorBreakTestRig(t, nil, "/v1/messages")
	raw := `{"system":"ORIG","messages":[]}`
	require.JSONEq(t, raw, postJSON(t, r, "/v1/messages", raw))
}

func TestArmorBreakMiddlewareSkipsWebSocketUpgrade(t *testing.T) {
	repo := &armorBreakFakeRepo{values: map[string]string{
		service.SettingKeyArmorBreakEnabled: "true",
		service.SettingKeyArmorBreakPersona: "R.txt",
	}}
	svc := service.NewArmorBreakService(repo, t.TempDir())
	_, err := svc.Store().Put("R.txt", "人格原文")
	require.NoError(t, err)

	r := newArmorBreakTestRig(t, svc, "/v1/responses")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"instructions":"ORIG"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(w, req)
	require.JSONEq(t, `{"instructions":"ORIG"}`, w.Body.String())
}
