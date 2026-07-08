// Package domain 定义 Ballast 核心领域模型。
// 结构体与 server/migrations 中的表一一对应。
package domain

import (
	"encoding/json"
	"time"
)

// SessionStatus 会话生命周期状态。
type SessionStatus string

const (
	SessionRunning   SessionStatus = "RUNNING"
	SessionSuspended SessionStatus = "SUSPENDED"
	SessionSuccess   SessionStatus = "SUCCESS"
	SessionFailed    SessionStatus = "FAILED"
)

// TriggerType 触发来源。
type TriggerType string

const (
	TriggerWebhook    TriggerType = "WEBHOOK"
	TriggerCron       TriggerType = "CRON"
	TriggerManualChat TriggerType = "MANUAL_CHAT"
)

// Session 对应 ballast_sessions 表。
type Session struct {
	SessionID   string        `json:"session_id"`
	Title       string        `json:"title"`
	TriggerType TriggerType   `json:"trigger_type"`
	Status      SessionStatus `json:"status"`
	AgentImage  string        `json:"agent_image"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// TriggerRule 对应 ballast_trigger_rules 表。
type TriggerRule struct {
	RuleID          string          `json:"rule_id"`
	Name            string          `json:"name"`
	IsActive        bool            `json:"is_active"`
	TriggerSource   string          `json:"trigger_source"`
	MatchExpression json.RawMessage `json:"match_expression"` // JSONB 原始字节
	BindSkills      []string        `json:"bind_skills"`
	AgentImage      string          `json:"agent_image"`
	PolicyGroup     string          `json:"policy_group"`
}

// MCPPlugin 对应 ballast_mcp_plugins 表。
// Env 存储注入沙箱 mcp_config.json 的环境变量，调用方必须避免写入长期生产密钥。
type MCPPlugin struct {
	PluginID  string            `json:"plugin_id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	IsActive  bool              `json:"is_active"`
	UpdatedBy string            `json:"updated_by"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Skill 对应 ballast_skills 表。
// MarkdownContent 是带 Frontmatter 的标准 OpenCode SKILL.md。
type Skill struct {
	SkillID         string    `json:"skill_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	TriggerWords    []string  `json:"trigger_words"`
	MarkdownContent string    `json:"markdown_content"`
	Version         int       `json:"version"`
	UpdatedBy       string    `json:"updated_by"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PolicyDecision 对应 ballast_audit_logs.policy_decision。
type PolicyDecision string

const (
	DecisionApprove PolicyDecision = "APPROVE"
	DecisionDeny    PolicyDecision = "DENY"
	DecisionSuspend PolicyDecision = "SUSPEND"
)

// AuditLog 对应 ballast_audit_logs 表。
type AuditLog struct {
	AuditID          int64          `json:"audit_id"`
	SessionID        string         `json:"session_id"`
	LoopIndex        int            `json:"loop_index"`
	ModelName        string         `json:"model_name"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	ExecutedCommand  string         `json:"executed_command"`
	PolicyDecision   PolicyDecision `json:"policy_decision"`
	Approver         string         `json:"approver"`
	RawTTYOutputPath string         `json:"raw_tty_output_path"`
	CreatedAt        time.Time      `json:"created_at"`
}

// SessionEvent 持久化 WebSocket 事件流，用于历史会话的 Reason Tree / TTY 回放。
type SessionEvent struct {
	EventID   int64           `json:"event_id"`
	SessionID string          `json:"session_id"`
	EventType string          `json:"type"`
	EventData json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}
