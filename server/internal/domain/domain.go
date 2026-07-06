// Package domain 定义 Ballast 核心领域模型。
// 结构体与 server/migrations 中的表一一对应。
package domain

import "time"

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
// v0.1 不启用自动化路由，结构体已定义供 v0.2 使用。
type TriggerRule struct {
	RuleID          string   `json:"rule_id"`
	Name            string   `json:"name"`
	IsActive        bool     `json:"is_active"`
	TriggerSource   string   `json:"trigger_source"`
	MatchExpression []byte   `json:"match_expression"` // JSONB 原始字节
	BindSkills      []string `json:"bind_skills"`
	AgentImage      string   `json:"agent_image"`
	PolicyGroup     string   `json:"policy_group"`
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
