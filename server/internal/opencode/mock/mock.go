// Package mock 提供 v0.1 占位的 OpenCode 引擎实现。
// 它不启动 HTTP server，而是在进程内按预设剧本异步推送事件流，
// 模拟一次 K8s CrashLoopBackOff 排障：捞日志 -> 拟修复 -> kubectl apply 触发灰名单。
//
// v0.2 将替换为 client.HTTPEngine，对接沙箱内 `opencode serve` 的 /event SSE。
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ballast/ballast-server/internal/opencode"
)

// MockEngine 实现 opencode.Engine。
type MockEngine struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
}

type sessionState struct {
	events chan opencode.Event
	stop   chan struct{}
}

// New 构造 MockEngine。
func New() *MockEngine {
	return &MockEngine{sessions: map[string]*sessionState{}}
}

// StartSession 创建一个会话，返回会话 ID。事件流在 Prompt 时开始推进。
func (m *MockEngine) StartSession(ctx context.Context, title string, opts opencode.SessionOpts) (string, error) {
	id := genID(title)
	st := &sessionState{
		events: make(chan opencode.Event, 64),
		stop:   make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[id] = st
	m.mu.Unlock()
	// 首事件
	st.events <- opencode.Event{
		Type:      "server.connected",
		Payload:   mustJSON(map[string]any{"server": "mock-opencode", "version": "0.1.0"}),
		Timestamp: time.Now(),
	}
	return id, nil
}

// Prompt 触发剧本推进：异步按时间间隔推送 reason.step / tool.call 事件，
// 最终吐出一条 `kubectl apply -f fixed_cm.yaml` 触发灰名单拦截。
func (m *MockEngine) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	m.mu.Lock()
	st, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	msgID := genID("msg")
	go m.runScript(sessionID, st, text)
	return msgID, nil
}

func (m *MockEngine) runScript(sessionID string, st *sessionState, prompt string) {
	script := []opencode.Event{
		{Type: "reason.step", Payload: mustJSON(map[string]any{"index": 1, "title": "列出异常 Pod", "thought": "执行 kubectl get pods 抓取 CrashLoopBackOff 现场"})},
		{Type: "tool.call", Payload: mustJSON(map[string]any{"tool": "bash", "command": "kubectl get pods -n prod", "stdout": "my-app-7b6f4-xz  0/1  CrashLoopBackOff  12  3m"})},
		{Type: "reason.step", Payload: mustJSON(map[string]any{"index": 2, "title": "捞取日志", "thought": "kubectl logs 找根因"})},
		{Type: "tool.call", Payload: mustJSON(map[string]any{"tool": "bash", "command": "kubectl logs my-app-7b6f4-xz -n prod", "stdout": "Error: configmap my-app-config not found"})},
		{Type: "reason.step", Payload: mustJSON(map[string]any{"index": 3, "title": "拟修复并应用", "thought": "生成修复 cm 并 kubectl apply 自愈"})},
		{Type: "tool.call", Payload: mustJSON(map[string]any{"tool": "bash", "command": "kubectl apply -f fixed_cm.yaml", "stdout": ""})},
		{Type: "message.completed", Payload: mustJSON(map[string]any{"role": "assistant", "text": "已尝试自愈，等待审批放行。"})},
	}
	for _, ev := range script {
		select {
		case <-st.stop:
			return
		case <-time.After(1200 * time.Millisecond):
		}
		ev.Timestamp = time.Now()
		select {
		case st.events <- ev:
		case <-st.stop:
			return
		}
	}
}

// Events 返回事件流 channel。会话结束后 channel 关闭。
func (m *MockEngine) Events(ctx context.Context, sessionID string) (<-chan opencode.Event, error) {
	m.mu.Lock()
	st, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	return st.events, nil
}

// InjectMCP 占位：记录即可。
func (m *MockEngine) InjectMCP(ctx context.Context, name string, cfg opencode.MCPConfig) error {
	return nil
}

// Stop 终止所有会话。
func (m *MockEngine) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, st := range m.sessions {
		select {
		case <-st.stop:
		default:
			close(st.stop)
		}
	}
	return nil
}

// StopSession 关闭单个会话（控制面销毁沙箱时调用）。
func (m *MockEngine) StopSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.sessions[sessionID]; ok {
		select {
		case <-st.stop:
		default:
			close(st.stop)
		}
		close(st.events)
		delete(m.sessions, sessionID)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func genID(prefix string) string {
	// 简单确定性 ID（v0.1 不需要强唯一性）。
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

var _ opencode.Engine = (*MockEngine)(nil)
