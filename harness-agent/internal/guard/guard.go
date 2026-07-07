// Package guard 拦截子进程待执行的命令，组装 CommandContext（spec 第 237-244 行），
// 上报控制面 PolicyEngine 求得 Decision，并据此放行/阻断/挂起。
//
// v0.1 通信走 HTTP JSON（控制面暴露 POST /api/internal/harness/report）。
// v0.2 切换为 gRPC（见 internal/proto/harness.proto）。
package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// CommandContext 对齐 spec/INIT.md 第 237-244 行。
type CommandContext struct {
	SessionID string   `json:"session_id"`
	User      string   `json:"user"`
	AgentName string   `json:"agent_name"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Env       []string `json:"env"`
	Unsafe    bool     `json:"unsafe"`
}

// Decision 对齐 spec 第 229-235 行。
type Decision string

const (
	Approve Decision = "APPROVE"
	Deny    Decision = "DENY"
	Suspend Decision = "SUSPEND"
)

// ReportResponse 控制面返回。
type ReportResponse struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}

// Reporter 上报命令到控制面并返回决策。
type Reporter interface {
	Report(ctx context.Context, cmdCtx CommandContext) (ReportResponse, error)
}

// HTTPReporter 通过 HTTP JSON 上报。
type HTTPReporter struct {
	Endpoint   string
	AgentName  string
	Token      string
	HTTPClient *http.Client
}

func NewHTTPReporter(endpoint, agentName, token string) *HTTPReporter {
	return &HTTPReporter{
		Endpoint:  endpoint,
		AgentName: agentName,
		Token:     token,
		// A SUSPEND request intentionally remains open until a human approves it.
		HTTPClient: &http.Client{},
	}
}

func (r *HTTPReporter) Report(ctx context.Context, cmdCtx CommandContext) (ReportResponse, error) {
	cmdCtx.AgentName = r.AgentName
	body, err := json.Marshal(cmdCtx)
	if err != nil {
		return ReportResponse{Decision: Deny}, fmt.Errorf("marshal command: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoint, bytes.NewReader(body))
	if err != nil {
		return ReportResponse{Decision: Deny}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Token)
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		// 上报失败：保守阻断。
		return ReportResponse{Decision: Deny}, fmt.Errorf("report command: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ReportResponse{Decision: Deny}, fmt.Errorf("control plane returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var out ReportResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ReportResponse{Decision: Deny}, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// Event is a normalized child-process event reported to the control plane.
type Event struct {
	SessionID string          `json:"session_id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
}

// HTTPEventReporter forwards reason/tool/message events to the control plane.
type HTTPEventReporter struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func NewHTTPEventReporter(endpoint, token string) *HTTPEventReporter {
	return &HTTPEventReporter{
		Endpoint:   endpoint,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (r *HTTPEventReporter) Report(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build event request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Token)
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("report event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("control plane returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

// ParseCommand 从一行输出中解析出待执行的 shell 命令。
// mock-opencode 的事件流里 tool.call 事件的 properties.command 字段包含命令。
// 输入示例：{"type":"tool.call","properties":{"tool":"bash","command":"kubectl apply -f fixed_cm.yaml"}}
// 解析失败返回空串。
func ParseCommand(line string) string {
	event, ok := ParseEvent(line)
	if !ok || event.Type != "tool.call" {
		return ""
	}
	var properties struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(event.Data, &properties); err != nil {
		return ""
	}
	return properties.Command
}

// ParseEvent parses one JSON-line event emitted by the controlled child.
func ParseEvent(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return Event{}, false
	}
	var ev struct {
		Type       string          `json:"type"`
		Properties json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return Event{}, false
	}
	if ev.Type == "" || len(ev.Properties) == 0 {
		return Event{}, false
	}
	return Event{Type: ev.Type, Data: ev.Properties}, true
}

// SplitCommand 将一条命令字符串拆为 command + args（朴素空格切分，v0.1 足够）。
func SplitCommand(raw string) (string, []string) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// AnalyzeCommand marks shell composition and interpreter wrappers as unsafe.
// The v0.1 gate intentionally prefers false positives over allowing a second,
// hidden command to inherit a whitelist decision from the first one.
func AnalyzeCommand(raw string) (string, []string, bool) {
	command, args := SplitCommand(raw)
	unsafe := strings.ContainsAny(raw, ";&|><`$()\r\n")
	switch command {
	case "sh", "bash", "zsh", "sudo", "env", "eval":
		unsafe = true
	}
	return command, args, unsafe
}

// RedactEnvironment keeps policy-relevant context without forwarding secret
// values to the control plane.
func RedactEnvironment(environment []string) []string {
	out := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		upperKey := strings.ToUpper(key)
		if strings.Contains(upperKey, "TOKEN") ||
			strings.Contains(upperKey, "SECRET") ||
			strings.Contains(upperKey, "PASSWORD") ||
			strings.Contains(upperKey, "CREDENTIAL") ||
			strings.HasSuffix(upperKey, "_KEY") {
			value = "<redacted>"
		}
		out = append(out, key+"="+value)
	}
	return out
}
