// Package policy 定义指令级安全策略引擎接口。
// 接口契约对齐 spec/INIT.md 第 224-249 行。
package policy

import "context"

// Decision 策略决策。
type Decision string

const (
	Approve Decision = "APPROVE"
	Deny    Decision = "DENY"
	Suspend Decision = "SUSPEND" // 触发人工审批断点
)

// CommandContext 输入：OpenCode 尝试执行的命令上下文（spec 第 237-244 行）。
type CommandContext struct {
	SessionID string   `json:"session_id"`
	User      string   `json:"user"`
	AgentName string   `json:"agent_name"`
	Command   string   `json:"command"` // Bash 原始命令
	Args      []string `json:"args"`
	Env       []string `json:"env"`
	Unsafe    bool     `json:"unsafe"`
}

// PolicyEngine 输入尝试执行的命令上下文，输出决策。
type PolicyEngine interface {
	EvaluateCommand(ctx context.Context, cmdCtx CommandContext) (Decision, error)
}
