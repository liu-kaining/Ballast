// Package opencode 定义 Ballast 与 OpenCode 引擎之间的抽象接口。
// v0.1 由 mock.MockEngine 实现；v0.2 由 client.HTTPEngine 实现，
// 通过 `opencode serve` 暴露的 OpenAPI 3.1 端点对接。
//
// 接口契约见 docs/opencode-protocol-research.md §5。
package opencode

import (
	"context"
	"encoding/json"
	"time"
)

// Engine 抽象 OpenCode 引擎的控制面。
type Engine interface {
	// StartSession 创建一个 OpenCode 会话，返回会话 ID。
	StartSession(ctx context.Context, title string, opts SessionOpts) (string, error)
	// Prompt 向会话发送一条用户消息，返回 AssistantMessage ID。
	// 事件通过 Events() 流异步推送。
	Prompt(ctx context.Context, sessionID string, text string) (string, error)
	// Events 返回 SSE 事件流的只读 channel。
	Events(ctx context.Context, sessionID string) (<-chan Event, error)
	// InjectMCP 动态挂载一个 MCP server 到引擎。
	InjectMCP(ctx context.Context, name string, config MCPConfig) error
	// Stop 终止引擎进程。
	Stop(ctx context.Context) error
}

type SessionOpts struct {
	ModelProvider string
	ModelID       string
	WorkingDir    string
	SkillDir      string // /workspace/.opencode/skills/
}

// Event 对应 opencode /event SSE 流中的一个事件。
type Event struct {
	Type      string          `json:"type"` // "server.connected", "reason.step", "tool.call", "message.completed", ...
	Payload   json.RawMessage `json:"properties"`
	Timestamp time.Time       `json:"timestamp"`
}

// MCPConfig 描述动态注入的 MCP server。
type MCPConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// ToolCallPayload 从 Event.Payload 解析 tool.call 事件的命令字段。
type ToolCallPayload struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
}

// ReasonStepPayload 从 Event.Payload 解析 reason.step 事件。
type ReasonStepPayload struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Thought string `json:"thought"`
}

// ParseToolCall 解析 tool.call 事件负载。
func ParseToolCall(e Event) (ToolCallPayload, bool) {
	if e.Type != "tool.call" {
		return ToolCallPayload{}, false
	}
	var p ToolCallPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return ToolCallPayload{}, false
	}
	return p, true
}

// ParseReasonStep 解析 reason.step 事件负载。
func ParseReasonStep(e Event) (ReasonStepPayload, bool) {
	if e.Type != "reason.step" {
		return ReasonStepPayload{}, false
	}
	var p ReasonStepPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return ReasonStepPayload{}, false
	}
	return p, true
}
