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
	HTTPClient *http.Client
}

func NewHTTPReporter(endpoint, agentName string) *HTTPReporter {
	return &HTTPReporter{
		Endpoint:   endpoint,
		AgentName:  agentName,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
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
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		// 上报失败：保守阻断。
		return ReportResponse{Decision: Deny}, fmt.Errorf("report command: %w", err)
	}
	defer resp.Body.Close()
	var out ReportResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ReportResponse{Decision: Deny}, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// ParseCommand 从一行输出中解析出待执行的 shell 命令。
// mock-opencode 的事件流里 tool.call 事件的 properties.command 字段包含命令。
// 输入示例：{"type":"tool.call","properties":{"tool":"bash","command":"kubectl apply -f fixed_cm.yaml"}}
// 解析失败返回空串。
func ParseCommand(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return ""
	}
	var ev struct {
		Type       string `json:"type"`
		Properties struct {
			Command string `json:"command"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return ""
	}
	if ev.Type != "tool.call" {
		return ""
	}
	return ev.Properties.Command
}

// SplitCommand 将一条命令字符串拆为 command + args（朴素空格切分，v0.1 足够）。
func SplitCommand(raw string) (string, []string) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
