// Package orchestrator owns the complete v0.1 session state machine.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/policy"
	"github.com/ballast/ballast-server/internal/runtime"
)

type SessionRepository interface {
	Create(context.Context, *domain.Session) error
	Get(context.Context, string) (*domain.Session, error)
	List(context.Context, domain.SessionStatus, int, int) ([]*domain.Session, error)
	UpdateStatus(context.Context, string, domain.SessionStatus) error
}

type AuditRepository interface {
	Append(context.Context, *domain.AuditLog) (int64, error)
}

type Manager struct {
	sessionsRepo SessionRepository
	auditRepo    AuditRepository
	runtime      runtime.SandboxRuntime
	policy       policy.PolicyEngine
	defaultImage string
	hub          *Hub

	mu       sync.RWMutex
	sessions map[string]*liveSession
}

type liveSession struct {
	mu       sync.RWMutex
	session  *domain.Session
	instance runtime.SandboxInstance
	pending  *pendingApproval
	ctx      context.Context
	cancel   context.CancelFunc
}

type pendingApproval struct {
	command  string
	result   chan approvalResult
	done     chan error
	approved bool
}

type approvalResult struct {
	approver string
}

func New(
	sessions SessionRepository,
	audit AuditRepository,
	sandboxRuntime runtime.SandboxRuntime,
	policyEngine policy.PolicyEngine,
	defaultImage string,
) *Manager {
	return &Manager{
		sessionsRepo: sessions,
		auditRepo:    audit,
		runtime:      sandboxRuntime,
		policy:       policyEngine,
		defaultImage: defaultImage,
		hub:          NewHub(),
		sessions:     make(map[string]*liveSession),
	}
}

func (m *Manager) CreateSession(ctx context.Context, title, agentImage string) (*domain.Session, error) {
	if agentImage == "" {
		agentImage = m.defaultImage
	}
	if agentImage != m.defaultImage {
		return nil, fmt.Errorf("agent image %q is not registered", agentImage)
	}

	now := time.Now().UTC()
	session := &domain.Session{
		SessionID:   genID(),
		Title:       title,
		TriggerType: domain.TriggerManualChat,
		Status:      domain.SessionRunning,
		AgentImage:  agentImage,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.sessionsRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(context.Background())
	live := &liveSession{session: session, ctx: sessionCtx, cancel: cancel}
	m.mu.Lock()
	m.sessions[session.SessionID] = live
	m.mu.Unlock()

	instance, err := m.runtime.Create(ctx, session.SessionID, agentImage, runtime.Mounts{})
	if err != nil {
		cancel()
		m.deleteLiveSession(session.SessionID)
		_ = m.sessionsRepo.UpdateStatus(context.Background(), session.SessionID, domain.SessionFailed)
		return nil, fmt.Errorf("create sandbox: %w", err)
	}
	live.mu.Lock()
	live.instance = instance
	live.mu.Unlock()

	m.hub.Broadcast(session.SessionID, EventEnvelope{
		Type: "session.started",
		Data: mustJSON(map[string]string{
			"session_id": session.SessionID,
			"sandbox_ip": instance.GetIP(),
		}),
	})
	return cloneSession(session), nil
}

func (m *Manager) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	if live := m.liveSession(id); live != nil {
		live.mu.RLock()
		defer live.mu.RUnlock()
		return cloneSession(live.session), nil
	}
	return m.sessionsRepo.Get(ctx, id)
}

func (m *Manager) ListSessions(ctx context.Context, status domain.SessionStatus, limit, offset int) ([]*domain.Session, error) {
	return m.sessionsRepo.List(ctx, status, limit, offset)
}

// HandleHarnessEvent forwards a trusted sandbox event and completes the
// session after the controlled child emits message.completed.
func (m *Manager) HandleHarnessEvent(ctx context.Context, id, eventType string, data json.RawMessage) error {
	live := m.liveSession(id)
	if live == nil {
		return fmt.Errorf("session %s is not active", id)
	}
	if !json.Valid(data) {
		return errors.New("event data is not valid JSON")
	}
	switch eventType {
	case "server.connected", "reason.step", "tool.call", "tool.result", "message.completed", "error":
	default:
		return fmt.Errorf("unsupported event type %q", eventType)
	}

	m.hub.Broadcast(id, EventEnvelope{Type: eventType, Data: data})
	terminalFailure := eventType == "error"
	if eventType == "tool.result" {
		var result struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return fmt.Errorf("decode tool result: %w", err)
		}
		terminalFailure = result.Decision == string(policy.Deny)
	}
	if terminalFailure {
		if err := m.setStatus(ctx, live, domain.SessionFailed); err != nil {
			return err
		}
		m.scheduleCleanup(id)
		return nil
	}
	if eventType == "message.completed" {
		if err := m.setStatus(ctx, live, domain.SessionSuccess); err != nil {
			return err
		}
		m.hub.Broadcast(id, EventEnvelope{
			Type: "session.completed",
			Data: mustJSON(map[string]string{"status": string(domain.SessionSuccess)}),
		})
		m.scheduleCleanup(id)
	}
	return nil
}

func (m *Manager) scheduleCleanup(id string) {
	time.AfterFunc(250*time.Millisecond, func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = m.finishSession(cleanupCtx, id, false)
	})
}

// EvaluateCommand evaluates one intercepted command. SUSPEND keeps the HTTP
// request open until Approve supplies a decision for this exact command.
func (m *Manager) EvaluateCommand(ctx context.Context, command policy.CommandContext) (policy.Decision, error) {
	live := m.liveSession(command.SessionID)
	if live == nil {
		return policy.Deny, fmt.Errorf("session %s is not active", command.SessionID)
	}
	if command.Command == "" {
		return policy.Deny, errors.New("command is empty")
	}
	command.Command = filepath.Base(command.Command)
	if containsShellControl(joinCommand(command.Command, command.Args)) {
		command.Unsafe = true
	}

	decision, err := m.policy.EvaluateCommand(ctx, command)
	if err != nil {
		return policy.Deny, fmt.Errorf("evaluate policy: %w", err)
	}
	auditID, err := m.auditRepo.Append(ctx, &domain.AuditLog{
		SessionID:       command.SessionID,
		LoopIndex:       1,
		ModelName:       command.AgentName,
		ExecutedCommand: joinCommand(command.Command, command.Args),
		PolicyDecision:  domain.PolicyDecision(decision),
	})
	if err != nil {
		return policy.Deny, fmt.Errorf("write policy audit: %w", err)
	}

	m.hub.Broadcast(command.SessionID, EventEnvelope{
		Type: "policy.decision",
		Data: mustJSON(map[string]any{
			"audit_id": auditID,
			"command":  joinCommand(command.Command, command.Args),
			"decision": string(decision),
		}),
	})

	switch decision {
	case policy.Approve:
		return policy.Approve, nil
	case policy.Deny:
		_ = m.setStatus(context.Background(), live, domain.SessionFailed)
		return policy.Deny, nil
	case policy.Suspend:
		return m.waitForApproval(ctx, live, command)
	default:
		return policy.Deny, fmt.Errorf("unsupported policy decision %q", decision)
	}
}

func (m *Manager) waitForApproval(ctx context.Context, live *liveSession, command policy.CommandContext) (policy.Decision, error) {
	pending := &pendingApproval{
		command: joinCommand(command.Command, command.Args),
		result:  make(chan approvalResult, 1),
		done:    make(chan error, 1),
	}
	live.mu.Lock()
	if live.pending != nil {
		live.mu.Unlock()
		return policy.Deny, errors.New("another command is already awaiting approval")
	}
	live.pending = pending
	live.mu.Unlock()

	if err := m.setStatus(context.Background(), live, domain.SessionSuspended); err != nil {
		m.clearPending(live, pending)
		return policy.Deny, err
	}

	select {
	case result := <-pending.result:
		_, auditErr := m.auditRepo.Append(context.Background(), &domain.AuditLog{
			SessionID:       command.SessionID,
			LoopIndex:       1,
			ModelName:       command.AgentName,
			ExecutedCommand: pending.command,
			PolicyDecision:  domain.DecisionApprove,
			Approver:        result.approver,
		})
		if auditErr == nil {
			auditErr = m.setStatus(context.Background(), live, domain.SessionRunning)
		}
		m.clearPending(live, pending)
		if auditErr != nil {
			pending.done <- auditErr
			return policy.Deny, auditErr
		}
		m.hub.Broadcast(command.SessionID, EventEnvelope{
			Type: "policy.resumed",
			Data: mustJSON(map[string]string{
				"approver": result.approver,
				"command":  pending.command,
			}),
		})
		pending.done <- nil
		return policy.Approve, nil
	case <-ctx.Done():
		m.clearPending(live, pending)
		return policy.Deny, ctx.Err()
	case <-live.ctx.Done():
		m.clearPending(live, pending)
		return policy.Deny, errors.New("session terminated while awaiting approval")
	}
}

func (m *Manager) Approve(ctx context.Context, id, approver string) error {
	live := m.liveSession(id)
	if live == nil {
		return fmt.Errorf("session %s is not active", id)
	}

	live.mu.Lock()
	pending := live.pending
	if pending == nil || pending.approved {
		live.mu.Unlock()
		return fmt.Errorf("session %s is not suspended", id)
	}
	pending.approved = true
	live.mu.Unlock()

	select {
	case pending.result <- approvalResult{approver: approver}:
	case <-ctx.Done():
		return ctx.Err()
	case <-live.ctx.Done():
		return errors.New("session terminated")
	}

	select {
	case err := <-pending.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-live.ctx.Done():
		return errors.New("session terminated")
	}
}

func (m *Manager) Destroy(ctx context.Context, id string) error {
	if m.liveSession(id) == nil {
		if _, err := m.sessionsRepo.Get(ctx, id); err != nil {
			return err
		}
		return nil
	}
	return m.finishSession(ctx, id, true)
}

func (m *Manager) finishSession(ctx context.Context, id string, userInitiated bool) error {
	live := m.liveSession(id)
	if live == nil {
		return nil
	}
	live.cancel()
	if err := m.runtime.Destroy(ctx, id); err != nil {
		_ = m.setStatus(context.Background(), live, domain.SessionFailed)
		return err
	}
	if userInitiated {
		live.mu.RLock()
		status := live.session.Status
		live.mu.RUnlock()
		if status != domain.SessionSuccess && status != domain.SessionFailed {
			status = domain.SessionFailed
			if err := m.setStatus(ctx, live, status); err != nil {
				return err
			}
		}
		m.hub.Broadcast(id, EventEnvelope{
			Type: "session.destroyed",
			Data: mustJSON(map[string]string{"status": string(status)}),
		})
	}
	m.deleteLiveSession(id)
	return nil
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var errs []error
	for _, id := range ids {
		if err := m.finishSession(ctx, id, true); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

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

func (m *Manager) setStatus(ctx context.Context, live *liveSession, status domain.SessionStatus) error {
	live.mu.RLock()
	id := live.session.SessionID
	live.mu.RUnlock()
	if err := m.sessionsRepo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}
	live.mu.Lock()
	live.session.Status = status
	live.session.UpdatedAt = time.Now().UTC()
	live.mu.Unlock()
	return nil
}

func (m *Manager) liveSession(id string) *liveSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *Manager) deleteLiveSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *Manager) clearPending(live *liveSession, pending *pendingApproval) {
	live.mu.Lock()
	if live.pending == pending {
		live.pending = nil
	}
	live.mu.Unlock()
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func joinCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	for _, arg := range args {
		command += " " + arg
	}
	return command
}

func containsShellControl(command string) bool {
	for _, character := range command {
		switch character {
		case ';', '&', '|', '>', '<', '`', '$', '(', ')', '\r', '\n':
			return true
		}
	}
	return false
}

func cloneSession(session *domain.Session) *domain.Session {
	cloned := *session
	return &cloned
}

func genID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}
