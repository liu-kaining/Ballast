// Package orchestrator owns the complete v0.1 session state machine.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/notify"
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
	ListBySession(context.Context, string, int) ([]*domain.AuditLog, error)
}

type EventRepository interface {
	Append(context.Context, *domain.SessionEvent) (int64, error)
	ListBySession(context.Context, string, int) ([]*domain.SessionEvent, error)
}

type SkillRepository interface {
	Get(context.Context, string) (*domain.Skill, error)
}

type MCPPluginRepository interface {
	Get(context.Context, string) (*domain.MCPPlugin, error)
}

type Manager struct {
	sessionsRepo   SessionRepository
	auditRepo      AuditRepository
	eventRepo      EventRepository
	skillsRepo     SkillRepository
	mcpRepo        MCPPluginRepository
	runtime        runtime.SandboxRuntime
	policy         policy.PolicyEngine
	notifier       notify.Notifier
	consoleBaseURL string
	defaultImage   string
	workspaceRoot  string
	hub            *Hub

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
	cleanup  func()
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

type Option func(*Manager)

func WithSkillRepository(repo SkillRepository) Option {
	return func(m *Manager) {
		m.skillsRepo = repo
	}
}

func WithEventRepository(repo EventRepository) Option {
	return func(m *Manager) {
		m.eventRepo = repo
	}
}

func WithMCPPluginRepository(repo MCPPluginRepository) Option {
	return func(m *Manager) {
		m.mcpRepo = repo
	}
}

func WithNotifier(notifier notify.Notifier, consoleBaseURL string) Option {
	return func(m *Manager) {
		m.notifier = notifier
		m.consoleBaseURL = strings.TrimRight(strings.TrimSpace(consoleBaseURL), "/")
	}
}

func WithWorkspaceRoot(root string) Option {
	return func(m *Manager) {
		m.workspaceRoot = strings.TrimSpace(root)
	}
}

type CreateSessionOptions struct {
	Title        string
	AgentImage   string
	TriggerType  domain.TriggerType
	SkillIDs     []string
	MCPPluginIDs []string
	WorkspaceDir string
}

type ManualCommandResult struct {
	Command        string                `json:"command"`
	Stdout         string                `json:"stdout"`
	Stderr         string                `json:"stderr"`
	Error          string                `json:"error,omitempty"`
	PolicyDecision domain.PolicyDecision `json:"policy_decision"`
	Approver       string                `json:"approver"`
}

func New(
	sessions SessionRepository,
	audit AuditRepository,
	sandboxRuntime runtime.SandboxRuntime,
	policyEngine policy.PolicyEngine,
	defaultImage string,
	options ...Option,
) *Manager {
	manager := &Manager{
		sessionsRepo: sessions,
		auditRepo:    audit,
		runtime:      sandboxRuntime,
		policy:       policyEngine,
		defaultImage: defaultImage,
		hub:          NewHub(),
		sessions:     make(map[string]*liveSession),
	}
	for _, option := range options {
		option(manager)
	}
	return manager
}

func (m *Manager) CreateSession(ctx context.Context, title, agentImage string, skillIDs ...string) (*domain.Session, error) {
	return m.CreateSessionWithOptions(ctx, CreateSessionOptions{
		Title:       title,
		AgentImage:  agentImage,
		TriggerType: domain.TriggerManualChat,
		SkillIDs:    skillIDs,
	})
}

func (m *Manager) CreateSessionWithOptions(ctx context.Context, opts CreateSessionOptions) (*domain.Session, error) {
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "未命名会话"
	}
	triggerType := opts.TriggerType
	if triggerType == "" {
		triggerType = domain.TriggerManualChat
	}
	agentImage := opts.AgentImage
	if agentImage == "" {
		agentImage = m.defaultImage
	}
	if agentImage != m.defaultImage {
		return nil, fmt.Errorf("agent image %q is not registered", agentImage)
	}

	now := time.Now().UTC()
	sessionID := genID()
	mounts, cleanup, err := m.prepareAssetMounts(ctx, sessionID, opts.WorkspaceDir, opts.SkillIDs, opts.MCPPluginIDs)
	if err != nil {
		return nil, err
	}
	session := &domain.Session{
		SessionID:   sessionID,
		Title:       title,
		TriggerType: triggerType,
		Status:      domain.SessionRunning,
		AgentImage:  agentImage,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.sessionsRepo.Create(ctx, session); err != nil {
		cleanup()
		return nil, fmt.Errorf("create session: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(context.Background())
	live := &liveSession{session: session, ctx: sessionCtx, cancel: cancel, cleanup: cleanup}
	m.mu.Lock()
	m.sessions[session.SessionID] = live
	m.mu.Unlock()

	instance, err := m.runtime.Create(ctx, session.SessionID, agentImage, mounts)
	if err != nil {
		cancel()
		cleanup()
		m.deleteLiveSession(session.SessionID)
		_ = m.sessionsRepo.UpdateStatus(context.Background(), session.SessionID, domain.SessionFailed)
		return nil, fmt.Errorf("create sandbox: %w", err)
	}
	live.mu.Lock()
	live.instance = instance
	live.mu.Unlock()

	m.publishEvent(context.Background(), session.SessionID, EventEnvelope{
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

func (m *Manager) ListAudit(ctx context.Context, id string, limit int) ([]*domain.AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return m.auditRepo.ListBySession(ctx, id, limit)
}

func (m *Manager) ListEvents(ctx context.Context, id string, limit int) ([]*domain.SessionEvent, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	if m.eventRepo == nil {
		return nil, nil
	}
	return m.eventRepo.ListBySession(ctx, id, limit)
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

	m.publishEvent(ctx, id, EventEnvelope{Type: eventType, Data: data})
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
		m.publishEvent(ctx, id, EventEnvelope{
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

	m.publishEvent(context.Background(), command.SessionID, EventEnvelope{
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
	m.notifyApprovalRequired(live, pending.command)

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
		m.publishEvent(context.Background(), command.SessionID, EventEnvelope{
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

func (m *Manager) ExecuteManualCommand(ctx context.Context, id string, argv []string, approver string) (ManualCommandResult, error) {
	live := m.liveSession(id)
	if live == nil {
		return ManualCommandResult{}, fmt.Errorf("session %s is not active", id)
	}
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return ManualCommandResult{}, errors.New("command argv is required")
	}
	if approver = strings.TrimSpace(approver); approver == "" {
		approver = "manual-takeover"
	}
	command := filepath.Base(strings.TrimSpace(argv[0]))
	args := make([]string, 0, len(argv)-1)
	for _, arg := range argv[1:] {
		if arg = strings.TrimSpace(arg); arg != "" {
			args = append(args, arg)
		}
	}
	rawCommand := joinCommand(command, args)
	cmdCtx := policy.CommandContext{
		SessionID: id,
		User:      approver,
		AgentName: "manual-takeover",
		Command:   command,
		Args:      args,
		Unsafe:    containsShellControl(rawCommand),
	}
	decision, err := m.policy.EvaluateCommand(ctx, cmdCtx)
	if err != nil {
		return ManualCommandResult{}, fmt.Errorf("evaluate manual command: %w", err)
	}
	if _, err := m.auditRepo.Append(ctx, &domain.AuditLog{
		SessionID:       id,
		LoopIndex:       1,
		ModelName:       "manual-takeover",
		ExecutedCommand: rawCommand,
		PolicyDecision:  domain.PolicyDecision(decision),
		Approver:        approver,
	}); err != nil {
		return ManualCommandResult{}, fmt.Errorf("write manual command audit: %w", err)
	}
	if decision == policy.Deny {
		result := ManualCommandResult{
			Command:        rawCommand,
			PolicyDecision: domain.DecisionDeny,
			Approver:       approver,
			Error:          "blocked by Ballast policy",
		}
		m.publishEvent(context.Background(), id, EventEnvelope{
			Type: "manual.command",
			Data: mustJSON(result),
		})
		return result, errors.New("manual command blocked by policy")
	}
	if decision == policy.Suspend {
		if _, err := m.auditRepo.Append(ctx, &domain.AuditLog{
			SessionID:       id,
			LoopIndex:       1,
			ModelName:       "manual-takeover",
			ExecutedCommand: rawCommand,
			PolicyDecision:  domain.DecisionApprove,
			Approver:        approver,
		}); err != nil {
			return ManualCommandResult{}, fmt.Errorf("write manual approval audit: %w", err)
		}
	}

	live.mu.RLock()
	instance := live.instance
	live.mu.RUnlock()
	if instance == nil {
		return ManualCommandResult{}, errors.New("sandbox instance is not ready")
	}
	stdout, stderr, execErr := instance.ExecuteCommand(ctx, append([]string{command}, args...))
	result := ManualCommandResult{
		Command:        rawCommand,
		Stdout:         truncateForEvent(string(stdout)),
		Stderr:         truncateForEvent(string(stderr)),
		PolicyDecision: domain.DecisionApprove,
		Approver:       approver,
	}
	if execErr != nil {
		result.Error = execErr.Error()
	}
	m.publishEvent(context.Background(), id, EventEnvelope{
		Type: "manual.command",
		Data: mustJSON(result),
	})
	return result, nil
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
		m.publishEvent(context.Background(), id, EventEnvelope{
			Type: "session.destroyed",
			Data: mustJSON(map[string]string{"status": string(status)}),
		})
	}
	if live.cleanup != nil {
		live.cleanup()
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

func (m *Manager) publishEvent(ctx context.Context, sessionID string, event EventEnvelope) {
	if m.eventRepo != nil {
		_, _ = m.eventRepo.Append(ctx, &domain.SessionEvent{
			SessionID: sessionID,
			EventType: event.Type,
			EventData: event.Data,
		})
	}
	m.hub.Broadcast(sessionID, event)
}

func (m *Manager) notifyApprovalRequired(live *liveSession, command string) {
	if m.notifier == nil {
		return
	}
	live.mu.RLock()
	sessionID := live.session.SessionID
	title := live.session.Title
	live.mu.RUnlock()
	consoleURL := ""
	if m.consoleBaseURL != "" {
		consoleURL = m.consoleBaseURL + "/sessions/" + sessionID
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.notifier.NotifyApprovalRequired(ctx, notify.ApprovalNotification{
			SessionID:  sessionID,
			Title:      title,
			Command:    command,
			ConsoleURL: consoleURL,
		})
	}()
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

func truncateForEvent(value string) string {
	const max = 32768
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...[truncated]"
}

func cloneSession(session *domain.Session) *domain.Session {
	cloned := *session
	return &cloned
}

func genID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}

var skillIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

var ErrInvalidCreateSessionOptions = errors.New("invalid create session options")

func (m *Manager) prepareAssetMounts(ctx context.Context, sessionID, workspaceDir string, skillIDs, mcpPluginIDs []string) (runtime.Mounts, func(), error) {
	mounts := runtime.Mounts{}
	cleanup := func() {}
	if workspaceDir != "" {
		workspaceMount, err := validateWorkspaceDir(workspaceDir)
		if err != nil {
			return runtime.Mounts{}, cleanup, err
		}
		mounts.WorkspaceDir = workspaceMount
	}
	if len(skillIDs) == 0 && len(mcpPluginIDs) == 0 {
		return mounts, cleanup, nil
	}
	if len(skillIDs) > 0 && m.skillsRepo == nil {
		return runtime.Mounts{}, cleanup, errors.New("skill repository is not configured")
	}
	if len(mcpPluginIDs) > 0 && m.mcpRepo == nil {
		return runtime.Mounts{}, cleanup, errors.New("mcp plugin repository is not configured")
	}
	root := m.workspaceRoot
	if root == "" {
		root = filepath.Join(os.TempDir(), "ballast", "sandboxes")
	}
	if !filepath.IsAbs(root) {
		return runtime.Mounts{}, cleanup, fmt.Errorf("workspace root must be absolute: %s", root)
	}
	sessionRoot := filepath.Join(root, sessionID)
	skillsRoot := filepath.Join(sessionRoot, "skills")
	mcpConfigPath := filepath.Join(sessionRoot, "mcp_config.json")
	cleanup = func() {
		_ = os.RemoveAll(sessionRoot)
	}
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return runtime.Mounts{}, cleanup, fmt.Errorf("create session workspace: %w", err)
	}

	if len(skillIDs) > 0 {
		if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
			return runtime.Mounts{}, cleanup, fmt.Errorf("create skills mount: %w", err)
		}
		if err := m.materializeSkills(ctx, skillsRoot, skillIDs); err != nil {
			cleanup()
			return runtime.Mounts{}, func() {}, err
		}
		mounts.SkillsDir = skillsRoot
	}

	if len(mcpPluginIDs) > 0 {
		if err := m.materializeMCPConfig(ctx, mcpConfigPath, mcpPluginIDs); err != nil {
			cleanup()
			return runtime.Mounts{}, func() {}, err
		}
		mounts.Extra = append(mounts.Extra, runtime.ExtraMount{
			Source:      mcpConfigPath,
			Destination: "/workspace/.opencode/mcp_config.json",
			ReadOnly:    true,
		})
	}
	return mounts, cleanup, nil
}

func validateWorkspaceDir(raw string) (string, error) {
	workspaceDir := filepath.Clean(strings.TrimSpace(raw))
	if workspaceDir == "." || workspaceDir == "" {
		return "", nil
	}
	if !filepath.IsAbs(workspaceDir) {
		return "", fmt.Errorf("%w: workspace_dir must be absolute: %s", ErrInvalidCreateSessionOptions, raw)
	}
	if workspaceDir == string(filepath.Separator) {
		return "", fmt.Errorf("%w: refusing to mount filesystem root as workspace_dir", ErrInvalidCreateSessionOptions)
	}
	info, err := os.Stat(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("%w: workspace_dir %s is not accessible: %v", ErrInvalidCreateSessionOptions, workspaceDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: workspace_dir must be a directory: %s", ErrInvalidCreateSessionOptions, workspaceDir)
	}
	return workspaceDir, nil
}

func (m *Manager) materializeSkills(ctx context.Context, skillsRoot string, skillIDs []string) error {
	seen := make(map[string]struct{}, len(skillIDs))
	for _, rawID := range skillIDs {
		skillID, err := normalizeAssetID(rawID)
		if err != nil {
			return fmt.Errorf("invalid skill id %q", rawID)
		}
		if _, ok := seen[skillID]; ok {
			continue
		}
		seen[skillID] = struct{}{}
		skill, err := m.skillsRepo.Get(ctx, skillID)
		if err != nil {
			return fmt.Errorf("load skill %s: %w", skillID, err)
		}
		skillDir := filepath.Join(skillsRoot, skillID)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("create skill dir %s: %w", skillID, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill.MarkdownContent), 0o644); err != nil {
			return fmt.Errorf("write skill %s: %w", skillID, err)
		}
	}
	return nil
}

func (m *Manager) materializeMCPConfig(ctx context.Context, path string, pluginIDs []string) error {
	type serverConfig struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env,omitempty"`
	}
	config := struct {
		MCPServers map[string]serverConfig `json:"mcpServers"`
	}{MCPServers: map[string]serverConfig{}}

	seen := make(map[string]struct{}, len(pluginIDs))
	for _, rawID := range pluginIDs {
		pluginID, err := normalizeAssetID(rawID)
		if err != nil {
			return fmt.Errorf("invalid mcp plugin id %q", rawID)
		}
		if _, ok := seen[pluginID]; ok {
			continue
		}
		seen[pluginID] = struct{}{}
		plugin, err := m.mcpRepo.Get(ctx, pluginID)
		if err != nil {
			return fmt.Errorf("load mcp plugin %s: %w", pluginID, err)
		}
		if !plugin.IsActive {
			continue
		}
		config.MCPServers[pluginID] = serverConfig{
			Command: plugin.Command,
			Args:    plugin.Args,
			Env:     plugin.Env,
		}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}
	return nil
}

func normalizeAssetID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if !skillIDPattern.MatchString(id) {
		return "", errors.New("invalid id")
	}
	return id, nil
}
