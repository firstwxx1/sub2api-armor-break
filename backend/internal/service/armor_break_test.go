package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// armorBreakTestSettingRepo 内存版 SettingRepository，覆盖接口全部方法。
type armorBreakTestSettingRepo struct {
	values map[string]string
}

func newArmorBreakTestSettingRepo() *armorBreakTestSettingRepo { return &armorBreakTestSettingRepo{values: map[string]string{}} }

func (r *armorBreakTestSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *armorBreakTestSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	s, err := r.Get(context.Background(), key)
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

func (r *armorBreakTestSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *armorBreakTestSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *armorBreakTestSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *armorBreakTestSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *armorBreakTestSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestArmorBreakShapeForPath(t *testing.T) {
	cases := map[string]ArmorBreakShape{
		"/v1/messages":                                      ArmorBreakShapeAnthropic,
		"/messages":                                         ArmorBreakShapeAnthropic,
		"/antigravity/v1/messages":                          ArmorBreakShapeAnthropic,
		"/v1/messages/count_tokens":                         ArmorBreakShapeNone,
		"/v1/chat/completions":                              ArmorBreakShapeOpenAIChat,
		"/chat/completions":                                 ArmorBreakShapeOpenAIChat,
		"/v1/responses":                                     ArmorBreakShapeOpenAIResponses,
		"/responses":                                        ArmorBreakShapeOpenAIResponses,
		"/backend-api/codex/responses":                      ArmorBreakShapeOpenAIResponses,
		"/v1beta/models/gemini-2.5-pro:generateContent":     ArmorBreakShapeGemini,
		"/v1beta/models/gemini-2.5-pro:streamGenerateContent": ArmorBreakShapeGemini,
		"/antigravity/v1beta/models/m:generateContent":      ArmorBreakShapeGemini,
		"/v1/models":                                        ArmorBreakShapeNone,
		"/v1/usage":                                         ArmorBreakShapeNone,
		"/v1/images/generations":                            ArmorBreakShapeNone,
	}
	for path, want := range cases {
		require.Equal(t, want, ArmorBreakShapeForPath(path), "path %s", path)
	}
}

func TestTransformAnthropicSystem(t *testing.T) {
	persona := "PERSONA-原文"

	// replace：无 system → 直接设置
	out := TransformBody(ArmorBreakShapeAnthropic, ArmorBreakModeReplace, persona, []byte(`{"model":"claude","messages":[]}`))
	require.Equal(t, persona, gjson.GetBytes(out, "system").String())

	// replace：覆盖原有 string system
	out = TransformBody(ArmorBreakShapeAnthropic, ArmorBreakModeReplace, persona, []byte(`{"system":"you are claude code","messages":[]}`))
	require.Equal(t, persona, gjson.GetBytes(out, "system").String())

	// prepend：string system 文本前置
	out = TransformBody(ArmorBreakShapeAnthropic, ArmorBreakModePrepend, persona, []byte(`{"system":"ORIG","messages":[]}`))
	require.Equal(t, persona+"\n\nORIG", gjson.GetBytes(out, "system").String())

	// prepend：array system 头部插块
	out = TransformBody(ArmorBreakShapeAnthropic, ArmorBreakModePrepend, persona, []byte(`{"system":[{"type":"text","text":"BLOCK0"}],"messages":[]}`))
	require.True(t, gjson.GetBytes(out, "system").IsArray())
	require.Equal(t, persona, gjson.GetBytes(out, "system.0.text").String())
	require.Equal(t, "BLOCK0", gjson.GetBytes(out, "system.1.text").String())

	// 非 JSON body 原样返回
	raw := []byte(`not json`)
	require.Equal(t, raw, TransformBody(ArmorBreakShapeAnthropic, ArmorBreakModeReplace, persona, raw))
}

func TestTransformOpenAIChatSystem(t *testing.T) {
	persona := "PERSONA-原文"

	// replace：首条 system → 替换 content
	out := TransformBody(ArmorBreakShapeOpenAIChat, ArmorBreakModeReplace, persona, []byte(`{"messages":[{"role":"system","content":"OLD"},{"role":"user","content":"hi"}]}`))
	require.Equal(t, persona, gjson.GetBytes(out, "messages.0.content").String())
	require.Equal(t, "hi", gjson.GetBytes(out, "messages.1.content").String())

	// replace：首条非 system → 头部插入
	out = TransformBody(ArmorBreakShapeOpenAIChat, ArmorBreakModeReplace, persona, []byte(`{"messages":[{"role":"user","content":"hi"}]}`))
	require.Equal(t, "system", gjson.GetBytes(out, "messages.0.role").String())
	require.Equal(t, persona, gjson.GetBytes(out, "messages.0.content").String())
	require.Equal(t, "user", gjson.GetBytes(out, "messages.1.role").String())

	// prepend：保留原 system 消息在其后
	out = TransformBody(ArmorBreakShapeOpenAIChat, ArmorBreakModePrepend, persona, []byte(`{"messages":[{"role":"system","content":"OLD"},{"role":"user","content":"hi"}]}`))
	require.Equal(t, persona, gjson.GetBytes(out, "messages.0.content").String())
	require.Equal(t, "OLD", gjson.GetBytes(out, "messages.1.content").String())

	// 无 messages → 不造结构，原样返回
	raw := []byte(`{"model":"gpt"}`)
	require.JSONEq(t, string(raw), string(TransformBody(ArmorBreakShapeOpenAIChat, ArmorBreakModeReplace, persona, raw)))
}

func TestTransformResponsesInstructions(t *testing.T) {
	persona := "PERSONA-原文"

	out := TransformBody(ArmorBreakShapeOpenAIResponses, ArmorBreakModeReplace, persona, []byte(`{"model":"gpt-5","instructions":"OLD","input":"hi"}`))
	require.Equal(t, persona, gjson.GetBytes(out, "instructions").String())

	out = TransformBody(ArmorBreakShapeOpenAIResponses, ArmorBreakModePrepend, persona, []byte(`{"instructions":"OLD","input":"hi"}`))
	require.Equal(t, persona+"\n\nOLD", gjson.GetBytes(out, "instructions").String())

	// 无 instructions + prepend → 直接设置
	out = TransformBody(ArmorBreakShapeOpenAIResponses, ArmorBreakModePrepend, persona, []byte(`{"input":"hi"}`))
	require.Equal(t, persona, gjson.GetBytes(out, "instructions").String())
}

func TestTransformGeminiSystemInstruction(t *testing.T) {
	persona := "PERSONA-原文"

	out := TransformBody(ArmorBreakShapeGemini, ArmorBreakModeReplace, persona, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	require.Equal(t, persona, gjson.GetBytes(out, "systemInstruction.parts.0.text").String())

	out = TransformBody(ArmorBreakShapeGemini, ArmorBreakModePrepend, persona, []byte(`{"systemInstruction":{"parts":[{"text":"OLD"}]},"contents":[]}`))
	require.Equal(t, persona+"\n\nOLD", gjson.GetBytes(out, "systemInstruction.parts.0.text").String())
}

func TestPersonaStore(t *testing.T) {
	dir := t.TempDir()
	store := NewPersonaStore(dir)

	// Put 自动补扩展名
	info, err := store.Put("R", "人格正文-v1")
	require.NoError(t, err)
	require.Equal(t, "R.txt", info.Name)

	// Get 命中
	content, err := store.Get("R.txt")
	require.NoError(t, err)
	require.Equal(t, "人格正文-v1", content)

	// 无扩展名 Get 同样解析
	content, err = store.Get("R")
	require.NoError(t, err)
	require.Equal(t, "人格正文-v1", content)

	// List
	list, err := store.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "R.txt", list[0].Name)

	// 缓存命中后修改文件内容但保持 size+mtime 困难，直接改内容（size 变）→ 重新读取
	require.NoError(t, os.WriteFile(filepath.Join(dir, "R.txt"), []byte("人格正文-v2-加长"), 0o644))
	content, err = store.Get("R.txt")
	require.NoError(t, err)
	require.Equal(t, "人格正文-v2-加长", content)

	// 路径穿越拒绝
	for _, bad := range []string{"../x", "..\\x", "a/b", "a\\b", "..", "x.exe", ".hidden/xx"} {
		_, err := store.Get(bad)
		require.Error(t, err, "name %q must be rejected", bad)
	}

	// Delete 幂等
	require.NoError(t, store.Delete("R.txt"))
	require.NoError(t, store.Delete("R.txt"))
	_, err = store.Get("R.txt")
	require.Error(t, err)
}

func TestArmorBreakServiceStateAndUpdate(t *testing.T) {
	repo := newArmorBreakTestSettingRepo()
	dir := t.TempDir()
	svc := NewArmorBreakService(repo, dir)
	ctx := context.Background()

	// 默认：关闭
	state, err := svc.State(ctx)
	require.NoError(t, err)
	require.False(t, state.Enabled)

	// 无文件时开启 → LoadErr fail-open
	enabled := true
	_, err = svc.Update(ctx, ArmorBreakUpdate{Enabled: &enabled})
	require.NoError(t, err)
	state, err = svc.State(ctx)
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.Error(t, state.LoadErr)
	require.Empty(t, state.Content)

	// 上传人格后选中
	_, err = svc.Store().Put("R.txt", "人格正文")
	require.NoError(t, err)
	_, err = svc.Update(ctx, ArmorBreakUpdate{Persona: armorBreakStrPtr("R.txt")})
	require.NoError(t, err)
	state, err = svc.State(ctx)
	require.NoError(t, err)
	require.NoError(t, state.LoadErr)
	require.Equal(t, "人格正文", state.Content)
	require.Equal(t, ArmorBreakModeReplace, state.Mode)

	// 非法 mode 拒绝
	_, err = svc.Update(ctx, ArmorBreakUpdate{Mode: armorBreakStrPtr("bogus")})
	require.Error(t, err)

	// 不存在的人格拒绝
	_, err = svc.Update(ctx, ArmorBreakUpdate{Persona: armorBreakStrPtr("ghost.txt")})
	require.Error(t, err)

	// 清除选中
	_, err = svc.Update(ctx, ArmorBreakUpdate{Persona: armorBreakStrPtr("")})
	require.NoError(t, err)
	state, err = svc.State(ctx)
	require.NoError(t, err)
	require.Empty(t, state.Persona)
	require.Error(t, state.LoadErr) // 开启但未选中人格

	// 设置变更持久化到 repo
	require.Equal(t, "true", repo.values[SettingKeyArmorBreakEnabled])

	// 状态缓存：3 秒内 repo 改动不反映
	repo.values[SettingKeyArmorBreakEnabled] = "false"
	state, err = svc.State(ctx)
	require.NoError(t, err)
	require.True(t, state.Enabled, "TTL cache should still hold old value")
	svc.Invalidate()
	state, err = svc.State(ctx)
	require.NoError(t, err)
	require.False(t, state.Enabled)
}

func TestPersonaStoreListEmptyDirMissing(t *testing.T) {
	store := NewPersonaStore(filepath.Join(t.TempDir(), "nonexistent"))
	list, err := store.List()
	require.NoError(t, err)
	require.Empty(t, list)
}

func armorBreakStrPtr(s string) *string { return &s }

// 防止 time 导入被优化掉（缓存 TTL 语义测试保留显式引用）。
var _ = time.Second
var _ = strings.TrimSpace
