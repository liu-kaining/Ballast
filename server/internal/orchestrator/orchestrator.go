// Package orchestrator 编排一次会话的完整生命周期：
//   会话落库 -> 启动 OpenCode 引擎 -> 订阅事件流 -> 转发到 WebSocket
//   -> tool.call 命令经 PolicyEngine 求值 -> APPROVE/DENY/SUSPEND
//   -> SUSPEND 时挂起等待人工 Approve -> Resume -> 审计落库 -> 销毁
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/opencode"
	"github.com/ballast/ballast-server/internal/policy"
	"github.com/ballast/ballast-server/internal/store"
)

// Manager 管理所有活跃会话。
type Manager struct {
	store   *store.Store
	engine  opencode.Engine
	policy  policy.PolicyEngine
	hub     *Hub

	mu       sync.Mutex
	sessions map[string]*liveSession
}

type liveSession struct {
	session  *domain.Session
	ocID     string // OpenCode 引擎侧会话 ID
	resumeCh chan string // approver
	suspendCh chan struct{}
	cancel   context.CancelFunc
}

// New 构造 Manager。
func New(s *store.Store, eng opencode.Engine, pol policy.PolicyEngine) *Manager {
	return &Manager{
		store:   s,
		engine:  eng,
		policy:  pol,
		hub:     NewHub(),
		sessions: map[string]*liveSession{},
	}
}

// Hub 返回 WebSocket 广播 hub，供 API 层订阅。
func (m *Manager) Hub() *Hub { return m.hub }

// CreateSession 创建会话：落库 + 启动引擎 + 事件循环。
func (m *Manager) CreateSession(ctx context.Context, title, agentImage string) (*domain.Session, error) {
	sess := &domain.Session{
		SessionID:   genID(),
		Title:       title,
		TriggerType: domain.TriggerManualChat,
		Status:      domain.SessionRunning,
		AgentImage:  agentImage,
	}
	if err := m.store.Sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	ocID, err := m.engine.StartSession(ctx, title, opencode.SessionOpts{})
	if err != nil {
		_ = m.store.Sessions.UpdateStatus(ctx, sess.SessionID, domain.SessionFailed)
		return nil, fmt.Errorf("start engine: %w", err)
	}

	lctx, cancel := context.WithCancel(context.Background())
	ls := &liveSession{
		session:   sess,
		ocID:      ocID,
		resumeCh:  make(chan string, 1),
		suspendCh: make(chan struct{}, 1),
		cancel:    cancel,
	}
	m.mu.Lock()
	m.sessions[sess.SessionID] = ls
	m.mu.Unlock()

	// 触发剧本推进
	if _, err := m.engine.Prompt(lctx, ocID, title); err != nil {
		return nil, fmt.Errorf("prompt: %w", err)
	}

	go m.eventLoop(lctx, ls)

	return sess, nil
}

// GetSession 从内存取活跃会话；若不在内存则回查 store。
func (m *Manager) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	m.mu.Lock()
	ls, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		return ls.session, nil
	}
	return m.store.Sessions.Get(ctx, id)
}

// ListSessions 列出会话。
func (m *Manager) ListSessions(ctx context.Context, status domain.SessionStatus, limit, offset int) ([]*domain.Session, error) {
	return m.store.Sessions.List(ctx, status, limit, offset)
}

// Approve 人工放行被挂起的会话。
func (m *Manager) Approve(ctx context.Context, id, approver string) error {
	m.mu.Lock()
	ls, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s not active", id)
	}
	select {
	case ls.resumeCh <- approver:
	default:
		return fmt.Errorf("session %s not suspended", id)
	}
	// 审计：记录放行
	_, _ = m.store.Audit.Append(ctx, &domain.AuditLog{
		SessionID:       id,
		LoopIndex:       1,
		ExecutedCommand: "human-approve",
		PolicyDecision:  domain.DecisionApprove,
		Approver:        approver,
	})
	_ = m.store.Sessions.UpdateStatus(ctx, id, domain.SessionRunning)
	return nil
}

// Destroy 销毁会话：停引擎、取消循环、更新状态。
func (m *Manager) Destroy(ctx context.Context, id string) error {
	m.mu.Lock()
	ls, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	ls.cancel()
	if stopper, ok := m.engine.(interface{ StopSession(string) }); ok {
		stopper.StopSession(ls.ocID)
	}
	_ = m.store.Sessions.UpdateStatus(ctx, id, domain.SessionSuccess)
	return nil
}

// eventLoop 消费引擎事件流，转发到 hub，并对 tool.call 做策略求值。
func (m *Manager) eventLoop(ctx context.Context, ls *liveSession) {
	events, err := m.engine.Events(ctx, ls.ocID)
	if err != nil {
		m.hub.Broadcast(ls.session.SessionID, EventEnvelope{Type: "error", Data: mustJSON(map[string]string{"error": err.Error()})})
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			// 转发给前端
			m.hub.Broadcast(ls.session.SessionID, EventEnvelope{
				Type: ev.Type,
				Data: ev.Payload,
			})
			// 命令拦截
			if tc, isTool := opencode.ParseToolCall(ev); isTool && tc.Command != "" {
				m.handleCommand(ctx, ls, tc)
			}
		}
	}
}

// handleCommand 对一条 tool.call 命令做策略求值并处理决策。
func (m *Manager) handleCommand(ctx context.Context, ls *liveSession, tc opencode.ToolCallPayload) {
	cmd, args := splitCmd(tc.Command)
	dec, err := m.policy.EvaluateCommand(ctx, policy.CommandContext{
		SessionID: ls.session.SessionID,
		User:      "opencode-agent",
		AgentName: "mock-opencode",
		Command:   cmd,
		Args:      args,
		Env:       nil,
	})
	if err != nil {
		dec = policy.Deny
	}
	audit := &domain.AuditLog{
		SessionID:       ls.session.SessionID,
		LoopIndex:       1,
		ModelName:       "mock-opencode",
		ExecutedCommand: tc.Command,
		PolicyDecision:  domain.PolicyDecision(dec),
	}
	auditID, _ := m.store.Audit.Append(ctx, audit)

	// 通知前端策略决策
	m.hub.Broadcast(ls.session.SessionID, EventEnvelope{
		Type: "policy.decision",
		Data: mustJSON(map[string]any{
			"audit_id": auditID,
			"command":  tc.Command,
			"decision": string(dec),
		}),
	})

	switch dec {
	case policy.Approve:
		// 放行
	case policy.Deny:
		_ = m.store.Sessions.UpdateStatus(ctx, ls.session.SessionID, domain.SessionFailed)
	case policy.Suspend:
		_ = m.store.Sessions.UpdateStatus(ctx, ls.session.SessionID, domain.SessionSuspended)
		// 阻塞等待放行
		select {
		case <-ctx.Done():
			return
		case approver := <-ls.resumeCh:
			m.hub.Broadcast(ls.session.SessionID, EventEnvelope{
				Type: "policy.resumed",
				Data: mustJSON(map[string]string{"approver": approver}),
			})
		}
	}
}

// EventEnvelope 是发给前端 WebSocket 的事件包装。
type EventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (m *Manager) Subscribe(sessionID string) <-chan EventEnvelope {
	return m.hub.Subscribe(sessionID)
}

func (m *Manager) Unsubscribe(sessionID string, ch <-chan EventEnvelope) {
	m.hub.Unsubscribe(sessionID, ch)
}

func splitCmd(raw string) (string, []string) {
	// 复用空格切分；与 harness-agent guard.SplitCommand 等价。
	var parts []string
	cur := ""
	for _, r := range raw {
		if r == ' ' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func genID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}
