package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 破甲（Armor Break）模块：在网关边缘把请求体里的 system 指令替换/前置为选中的
// 人格文件内容。下游客户端无需任何补丁——只要调用了本网关配置好的模型，
// 请求到达上游时 system 已经是人格原文。
//
// 三种设置存于 settings 表（缺失即默认值）：
//   - armor_break_enabled  "true"/"false"（默认 false）
//   - armor_break_persona  人格文件名（personas 目录内，如 "R.txt"）
//   - armor_break_mode     "replace"（默认，整段替换 system）| "prepend"（人格前置，保留原 system）
//
// 人格文件存于人格目录（默认 ./personas，环境变量 ARMOR_BREAK_PERSONA_DIR 覆盖），
// 支持 .txt / .md，热生效：文件变更按 mtime+size 失效缓存，设置变更 3 秒内被所有请求看到。

const (
	SettingKeyArmorBreakEnabled = "armor_break_enabled"
	SettingKeyArmorBreakPersona = "armor_break_persona"
	SettingKeyArmorBreakMode    = "armor_break_mode"

	ArmorBreakModeReplace = "replace"
	ArmorBreakModePrepend = "prepend"

	armorBreakStateTTL      = 3 * time.Second
	armorBreakMaxPersonaLen = 512 * 1024 // 人格文件上限 512KB
)

// ArmorBreakShape 标识入口协议形态，决定 system 字段的改写方式。
type ArmorBreakShape int

const (
	ArmorBreakShapeNone ArmorBreakShape = iota
	ArmorBreakShapeAnthropic
	ArmorBreakShapeOpenAIChat
	ArmorBreakShapeOpenAIResponses
	ArmorBreakShapeGemini
)

// ArmorBreakShapeForPath 按请求路径判定协议形态。非生成类端点（count_tokens、
// models、usage 等）返回 None，中间件直接放行。
func ArmorBreakShapeForPath(path string) ArmorBreakShape {
	switch {
	case strings.Contains(path, "count_tokens"):
		return ArmorBreakShapeNone
	case strings.Contains(path, "/messages"):
		return ArmorBreakShapeAnthropic
	case strings.HasSuffix(path, "/chat/completions"):
		return ArmorBreakShapeOpenAIChat
	case strings.Contains(path, "/responses"):
		return ArmorBreakShapeOpenAIResponses
	case strings.Contains(path, "generateContent"), strings.Contains(path, "GenerateContent"):
		// /v1beta/models/{m}:generateContent、:streamGenerateContent 及
		// /antigravity/v1beta 同构路径。
		return ArmorBreakShapeGemini
	default:
		return ArmorBreakShapeNone
	}
}

// PersonaInfo 人格文件元信息（列表用）。
type PersonaInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PersonaStore 人格文件仓库：目录扫描 + 按 mtime/size 失效的内容缓存。
// 名称严格校验，拒绝路径穿越。
type PersonaStore struct {
	dir string

	mu    sync.RWMutex
	cache map[string]*personaEntry
}

type personaEntry struct {
	content string
	size    int64
	modTime time.Time
}

// NewPersonaStore 创建人格仓库。dir 为空时读 ARMOR_BREAK_PERSONA_DIR，再缺省 ./personas。
func NewPersonaStore(dir string) *PersonaStore {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("ARMOR_BREAK_PERSONA_DIR"))
	}
	if dir == "" {
		dir = "personas"
	}
	return &PersonaStore{dir: dir, cache: make(map[string]*personaEntry)}
}

// Dir 返回人格目录路径（管理面展示/诊断用）。
func (s *PersonaStore) Dir() string { return s.dir }

// validatePersonaName 校验人格文件名：仅允许裸文件名（可不带扩展名，自动补 .txt），
// 扩展名限 .txt/.md，拒绝一切路径分隔与穿越。
func validatePersonaName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("persona name is required")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid persona name %q", name)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		name += ".txt"
		ext = ".txt"
	}
	if ext != ".txt" && ext != ".md" {
		return "", fmt.Errorf("persona name %q must end with .txt or .md", name)
	}
	return name, nil
}

// List 扫描人格目录，按名称排序返回人格清单。目录不存在返回空清单（不视为错误）。
func (s *PersonaStore) List() ([]PersonaInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PersonaInfo{}, nil
		}
		return nil, fmt.Errorf("read persona dir: %w", err)
	}
	out := make([]PersonaInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".txt" && ext != ".md" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, PersonaInfo{Name: name, Size: info.Size(), UpdatedAt: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get 读取人格内容，带 mtime+size 缓存：文件未变直接命中内存。
func (s *PersonaStore) Get(name string) (string, error) {
	name, err := validatePersonaName(name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.dir, name)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("persona %q not found", name)
		}
		return "", fmt.Errorf("stat persona: %w", err)
	}
	if info.Size() > armorBreakMaxPersonaLen {
		return "", fmt.Errorf("persona %q exceeds %d bytes", name, armorBreakMaxPersonaLen)
	}

	s.mu.RLock()
	cached := s.cache[name]
	s.mu.RUnlock()
	if cached != nil && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.content, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read persona: %w", err)
	}
	content := string(raw)
	s.mu.Lock()
	s.cache[name] = &personaEntry{content: content, size: info.Size(), modTime: info.ModTime()}
	s.mu.Unlock()
	return content, nil
}

// Put 写入/更新人格文件并刷新缓存。
func (s *PersonaStore) Put(name, content string) (PersonaInfo, error) {
	name, err := validatePersonaName(name)
	if err != nil {
		return PersonaInfo{}, err
	}
	if len(content) > armorBreakMaxPersonaLen {
		return PersonaInfo{}, fmt.Errorf("persona content exceeds %d bytes", armorBreakMaxPersonaLen)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return PersonaInfo{}, fmt.Errorf("create persona dir: %w", err)
	}
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return PersonaInfo{}, fmt.Errorf("write persona: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return PersonaInfo{}, fmt.Errorf("stat persona after write: %w", err)
	}
	s.mu.Lock()
	s.cache[name] = &personaEntry{content: content, size: info.Size(), modTime: info.ModTime()}
	s.mu.Unlock()
	return PersonaInfo{Name: name, Size: info.Size(), UpdatedAt: info.ModTime()}, nil
}

// Delete 删除人格文件并清缓存。文件不存在视为成功（幂等）。
func (s *PersonaStore) Delete(name string) error {
	name, err := validatePersonaName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, name)
	s.mu.Unlock()
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete persona: %w", err)
	}
	return nil
}

// ArmorBreakState 破甲运行时状态快照（中间件热路径消费）。
type ArmorBreakState struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Persona string `json:"persona"`
	// Content 已加载的人格正文。LoadErr 非空时为空串。
	Content string `json:"-"`
	// LoadErr 人格加载失败原因（启用但文件缺失/损坏）。中间件据此 fail-open 放行。
	LoadErr error `json:"-"`
}

// ArmorBreakService 破甲核心服务：设置读写 + 人格加载 + 请求体改写。
type ArmorBreakService struct {
	repo  SettingRepository
	store *PersonaStore

	stateMu sync.Mutex
	state   *ArmorBreakState
	stateAt time.Time
}

// NewArmorBreakService 构造破甲服务。personaDir 为空走 env/默认目录。
func NewArmorBreakService(repo SettingRepository, personaDir string) *ArmorBreakService {
	return &ArmorBreakService{
		repo:  repo,
		store: NewPersonaStore(personaDir),
	}
}

// Store 暴露人格仓库（管理面 CRUD 用）。
func (s *ArmorBreakService) Store() *PersonaStore { return s.store }

// State 读取破甲状态，3 秒 TTL 缓存避免每请求打库。
// 设置读取失败时返回错误；人格加载失败不返回错误而是置 LoadErr（fail-open）。
func (s *ArmorBreakService) State(ctx context.Context) (*ArmorBreakState, error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state != nil && time.Since(s.stateAt) < armorBreakStateTTL {
		return s.state, nil
	}
	values, err := s.repo.GetMultiple(ctx, []string{
		SettingKeyArmorBreakEnabled,
		SettingKeyArmorBreakPersona,
		SettingKeyArmorBreakMode,
	})
	if err != nil {
		return nil, fmt.Errorf("load armor break settings: %w", err)
	}
	state := &ArmorBreakState{
		Enabled: strings.EqualFold(strings.TrimSpace(values[SettingKeyArmorBreakEnabled]), "true"),
		Mode:    strings.TrimSpace(values[SettingKeyArmorBreakMode]),
		Persona: strings.TrimSpace(values[SettingKeyArmorBreakPersona]),
	}
	if state.Mode == "" {
		state.Mode = ArmorBreakModeReplace
	}
	if state.Enabled {
		if state.Persona == "" {
			state.LoadErr = errors.New("armor break enabled but no persona selected")
		} else if content, err := s.store.Get(state.Persona); err != nil {
			state.LoadErr = err
		} else {
			state.Content = content
		}
	}
	s.state = state
	s.stateAt = time.Now()
	return state, nil
}

// Invalidate 立即失效状态缓存（设置更新后调用）。
func (s *ArmorBreakService) Invalidate() {
	s.stateMu.Lock()
	s.state = nil
	s.stateMu.Unlock()
}

// ArmorBreakUpdate 管理面更新请求（nil 字段不动）。
type ArmorBreakUpdate struct {
	Enabled *bool   `json:"enabled"`
	Mode    *string `json:"mode"`
	Persona *string `json:"persona"`
}

// Update 校验并持久化破甲设置，随后失效缓存（下个请求即生效）。
func (s *ArmorBreakService) Update(ctx context.Context, upd ArmorBreakUpdate) (*ArmorBreakState, error) {
	patch := make(map[string]string, 3)
	if upd.Mode != nil {
		mode := strings.TrimSpace(*upd.Mode)
		if mode != ArmorBreakModeReplace && mode != ArmorBreakModePrepend {
			return nil, fmt.Errorf("mode must be %q or %q", ArmorBreakModeReplace, ArmorBreakModePrepend)
		}
		patch[SettingKeyArmorBreakMode] = mode
	}
	if upd.Persona != nil {
		if strings.TrimSpace(*upd.Persona) == "" {
			// 空串 = 清除选中人格。
			patch[SettingKeyArmorBreakPersona] = ""
		} else {
			persona, err := validatePersonaName(*upd.Persona)
			if err != nil {
				return nil, err
			}
			if _, err := s.store.Get(persona); err != nil {
				return nil, err
			}
			patch[SettingKeyArmorBreakPersona] = persona
		}
	}
	if upd.Enabled != nil {
		patch[SettingKeyArmorBreakEnabled] = strconv.FormatBool(*upd.Enabled)
	}
	if len(patch) > 0 {
		if err := s.repo.SetMultiple(ctx, patch); err != nil {
			return nil, fmt.Errorf("save armor break settings: %w", err)
		}
	}
	s.Invalidate()
	return s.State(ctx)
}

// TransformBody 按协议形态把 body 中的 system 指令改写为 persona。
// mode=replace 整段替换；mode=prepend 人格前置、保留原 system。
// 非 JSON 对象 body 原样返回。
func TransformBody(shape ArmorBreakShape, mode, persona string, body []byte) []byte {
	if shape == ArmorBreakShapeNone || persona == "" {
		return body
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body
	}
	if mode == "" {
		mode = ArmorBreakModeReplace
	}
	var out []byte
	var err error
	switch shape {
	case ArmorBreakShapeAnthropic:
		out, err = transformAnthropicSystem(mode, persona, body)
	case ArmorBreakShapeOpenAIChat:
		out, err = transformOpenAIChatSystem(mode, persona, body)
	case ArmorBreakShapeOpenAIResponses:
		out, err = transformResponsesInstructions(mode, persona, body)
	case ArmorBreakShapeGemini:
		out, err = transformGeminiSystemInstruction(mode, persona, body)
	}
	if err != nil || len(out) == 0 {
		return body
	}
	return out
}

// transformAnthropicSystem 改写 Anthropic /v1/messages 的顶层 system 字段。
// replace：直接设为 persona 字符串。prepend：string 则文本前置；array 则头部插入
// {"type":"text"} 块。
func transformAnthropicSystem(mode, persona string, body []byte) ([]byte, error) {
	if mode == ArmorBreakModeReplace {
		return sjson.SetBytes(body, "system", persona)
	}
	sys := gjson.GetBytes(body, "system")
	switch {
	case !sys.Exists():
		return sjson.SetBytes(body, "system", persona)
	case sys.Type == gjson.String:
		return sjson.SetBytes(body, "system", persona+"\n\n"+sys.String())
	case sys.IsArray():
		block := `{"type":"text","text":` + strconv.Quote(persona) + `}`
		raw := strings.TrimSpace(sys.Raw)
		if len(raw) >= 2 {
			return sjson.SetRawBytes(body, "system", []byte(`[`+block+`,`+raw[1:]))
		}
		return sjson.SetBytes(body, "system", persona)
	default:
		return sjson.SetBytes(body, "system", persona)
	}
}

// transformOpenAIChatSystem 改写 OpenAI chat/completions 的 messages。
// replace：messages[0].role==system 时替换其 content，否则头部插入新 system 消息。
// prepend：无条件在头部插入 persona system 消息（保留原 system 消息在其后）。
func transformOpenAIChatSystem(mode, persona string, body []byte) ([]byte, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		// 无 messages（异常请求），交由下游报错，不造结构。
		return body, nil
	}
	if mode == ArmorBreakModeReplace && gjson.GetBytes(body, "messages.0.role").String() == "system" {
		return sjson.SetBytes(body, "messages.0.content", persona)
	}
	msg := `{"role":"system","content":` + strconv.Quote(persona) + `}`
	raw := strings.TrimSpace(messages.Raw)
	if len(raw) < 2 {
		return sjson.SetBytes(body, "messages", []any{map[string]any{"role": "system", "content": persona}})
	}
	return sjson.SetRawBytes(body, "messages", []byte(`[`+msg+`,`+raw[1:]))
}

// transformResponsesInstructions 改写 OpenAI Responses API 的 instructions 字段。
func transformResponsesInstructions(mode, persona string, body []byte) ([]byte, error) {
	if mode == ArmorBreakModePrepend {
		if existing := gjson.GetBytes(body, "instructions"); existing.Exists() && existing.String() != "" {
			return sjson.SetBytes(body, "instructions", persona+"\n\n"+existing.String())
		}
	}
	return sjson.SetBytes(body, "instructions", persona)
}

// transformGeminiSystemInstruction 改写 Gemini generateContent 的 systemInstruction。
// prepend 时与原 parts[0].text 拼接。
func transformGeminiSystemInstruction(mode, persona string, body []byte) ([]byte, error) {
	text := persona
	if mode == ArmorBreakModePrepend {
		if existing := gjson.GetBytes(body, "systemInstruction.parts.0.text"); existing.Exists() && existing.String() != "" {
			text = persona + "\n\n" + existing.String()
		}
	}
	return sjson.SetBytes(body, "systemInstruction", map[string]any{
		"parts": []any{map[string]any{"text": text}},
	})
}
