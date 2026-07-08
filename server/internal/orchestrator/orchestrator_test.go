package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/policy"
	"github.com/ballast/ballast-server/internal/runtime"
)

type memorySessions struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
}

func newMemorySessions() *memorySessions {
	return &memorySessions{sessions: make(map[string]*domain.Session)}
}

func (m *memorySessions) Create(_ context.Context, session *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.SessionID] = cloneSession(session)
	return nil
}

func (m *memorySessions) Get(_ context.Context, id string) (*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[id]
	if session == nil {
		return nil, errors.New("not found")
	}
	return cloneSession(session), nil
}

func (m *memorySessions) List(_ context.Context, _ domain.SessionStatus, _, _ int) ([]*domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		out = append(out, cloneSession(session))
	}
	return out, nil
}

func (m *memorySessions) UpdateStatus(_ context.Context, id string, status domain.SessionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[id] == nil {
		return errors.New("not found")
	}
	m.sessions[id].Status = status
	m.sessions[id].UpdatedAt = time.Now()
	return nil
}

type memoryAudit struct {
	mu   sync.Mutex
	logs []*domain.AuditLog
}

func (m *memoryAudit) Append(_ context.Context, log *domain.AuditLog) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cloned := *log
	m.logs = append(m.logs, &cloned)
	return int64(len(m.logs)), nil
}

func (m *memoryAudit) ListBySession(_ context.Context, id string, limit int) ([]*domain.AuditLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.AuditLog
	for _, log := range m.logs {
		if log.SessionID == id {
			cloned := *log
			out = append(out, &cloned)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

type memoryEvents struct {
	mu     sync.Mutex
	events []*domain.SessionEvent
}

func (m *memoryEvents) Append(_ context.Context, event *domain.SessionEvent) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cloned := *event
	cloned.EventID = int64(len(m.events) + 1)
	m.events = append(m.events, &cloned)
	return cloned.EventID, nil
}

func (m *memoryEvents) ListBySession(_ context.Context, id string, limit int) ([]*domain.SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.SessionEvent
	for _, event := range m.events {
		if event.SessionID == id {
			cloned := *event
			out = append(out, &cloned)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

type memorySkills map[string]*domain.Skill

func (m memorySkills) Get(_ context.Context, id string) (*domain.Skill, error) {
	skill := m[id]
	if skill == nil {
		return nil, errors.New("not found")
	}
	cloned := *skill
	return &cloned, nil
}

type memoryMCPPlugins map[string]*domain.MCPPlugin

func (m memoryMCPPlugins) Get(_ context.Context, id string) (*domain.MCPPlugin, error) {
	plugin := m[id]
	if plugin == nil {
		return nil, errors.New("not found")
	}
	cloned := *plugin
	return &cloned, nil
}

type fakeRuntime struct {
	mu        sync.Mutex
	created   []string
	destroyed []string
	mounts    []runtime.Mounts
}

func (f *fakeRuntime) Create(_ context.Context, id, _ string, mounts runtime.Mounts) (runtime.SandboxInstance, error) {
	f.mu.Lock()
	f.created = append(f.created, id)
	f.mounts = append(f.mounts, mounts)
	f.mu.Unlock()
	return fakeInstance{id: id}, nil
}
func (f *fakeRuntime) InjectJITCredential(context.Context, string, string) error { return nil }
func (f *fakeRuntime) Destroy(_ context.Context, id string) error {
	f.mu.Lock()
	f.destroyed = append(f.destroyed, id)
	f.mu.Unlock()
	return nil
}

type fakeInstance struct{ id string }

func (f fakeInstance) GetID() string { return f.id }
func (f fakeInstance) GetIP() string { return "127.0.0.2" }
func (f fakeInstance) ExecuteCommand(context.Context, []string) ([]byte, []byte, error) {
	return []byte("manual stdout"), nil, nil
}

type commandPolicy struct{}

func (commandPolicy) EvaluateCommand(_ context.Context, command policy.CommandContext) (policy.Decision, error) {
	if command.Command == "rm" {
		return policy.Deny, nil
	}
	if len(command.Args) > 0 && command.Args[0] == "get" {
		return policy.Approve, nil
	}
	return policy.Suspend, nil
}

func newTestManager() (*Manager, *memorySessions, *memoryAudit, *fakeRuntime) {
	sessions := newMemorySessions()
	audit := &memoryAudit{}
	sandbox := &fakeRuntime{}
	manager := New(sessions, audit, sandbox, commandPolicy{}, "sandbox:test")
	return manager, sessions, audit, sandbox
}

func TestCreateSessionMaterializesSelectedSkills(t *testing.T) {
	sessions := newMemorySessions()
	audit := &memoryAudit{}
	sandbox := &fakeRuntime{}
	root := t.TempDir()
	manager := New(
		sessions,
		audit,
		sandbox,
		commandPolicy{},
		"sandbox:test",
		WithWorkspaceRoot(root),
		WithSkillRepository(memorySkills{
			"k8s-debug": {
				SkillID:         "k8s-debug",
				Name:            "K8s Debug",
				MarkdownContent: "---\nname: k8s-debug\n---\n# Debug\n",
			},
		}),
	)

	session, err := manager.CreateSession(context.Background(), "with skill", "", "k8s-debug")
	if err != nil {
		t.Fatal(err)
	}
	sandbox.mu.Lock()
	if len(sandbox.mounts) != 1 || sandbox.mounts[0].SkillsDir == "" {
		t.Fatalf("mounts = %#v", sandbox.mounts)
	}
	skillsDir := sandbox.mounts[0].SkillsDir
	sandbox.mu.Unlock()

	content, err := os.ReadFile(filepath.Join(skillsDir, "k8s-debug", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "" {
		t.Fatal("empty materialized skill")
	}
	if err := manager.Destroy(context.Background(), session.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, session.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("session workspace was not cleaned up: %v", err)
	}
}

func TestCreateSessionMaterializesMCPConfig(t *testing.T) {
	sessions := newMemorySessions()
	audit := &memoryAudit{}
	sandbox := &fakeRuntime{}
	root := t.TempDir()
	manager := New(
		sessions,
		audit,
		sandbox,
		commandPolicy{},
		"sandbox:test",
		WithWorkspaceRoot(root),
		WithMCPPluginRepository(memoryMCPPlugins{
			"prometheus": {
				PluginID: "prometheus",
				Name:     "Prometheus MCP",
				Command:  "prometheus-mcp",
				Args:     []string{"--stdio"},
				Env:      map[string]string{"PROM_URL": "http://prometheus:9090"},
				IsActive: true,
			},
		}),
	)

	session, err := manager.CreateSessionWithOptions(context.Background(), CreateSessionOptions{
		Title:        "with mcp",
		MCPPluginIDs: []string{"prometheus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.mu.Lock()
	if len(sandbox.mounts) != 1 || len(sandbox.mounts[0].Extra) != 1 {
		t.Fatalf("mounts = %#v", sandbox.mounts)
	}
	mcpMount := sandbox.mounts[0].Extra[0]
	sandbox.mu.Unlock()
	if mcpMount.Destination != "/workspace/.opencode/mcp_config.json" || !mcpMount.ReadOnly {
		t.Fatalf("mcp mount = %#v", mcpMount)
	}
	content, err := os.ReadFile(mcpMount.Source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "prometheus-mcp") {
		t.Fatalf("mcp config content = %s", string(content))
	}
	if err := manager.Destroy(context.Background(), session.SessionID); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSessionMountsWorkspaceDir(t *testing.T) {
	manager, _, _, sandbox := newTestManager()
	workspaceDir := t.TempDir()
	session, err := manager.CreateSessionWithOptions(context.Background(), CreateSessionOptions{
		Title:        "with workspace",
		WorkspaceDir: workspaceDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionID == "" {
		t.Fatal("empty session id")
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if len(sandbox.mounts) != 1 || sandbox.mounts[0].WorkspaceDir != workspaceDir {
		t.Fatalf("mounts = %#v, want workspace %s", sandbox.mounts, workspaceDir)
	}
}

func TestCreateSessionRejectsInvalidWorkspaceDir(t *testing.T) {
	manager, _, _, sandbox := newTestManager()
	for _, workspaceDir := range []string{"relative/project", string(filepath.Separator)} {
		t.Run(workspaceDir, func(t *testing.T) {
			_, err := manager.CreateSessionWithOptions(context.Background(), CreateSessionOptions{
				Title:        "bad workspace",
				WorkspaceDir: workspaceDir,
			})
			if !errors.Is(err, ErrInvalidCreateSessionOptions) {
				t.Fatalf("err = %v, want ErrInvalidCreateSessionOptions", err)
			}
		})
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if len(sandbox.created) != 0 {
		t.Fatalf("sandbox should not be created for invalid workspace: %v", sandbox.created)
	}
}

func TestSessionLifecycleRequiresCommandSpecificApproval(t *testing.T) {
	manager, _, audit, _ := newTestManager()
	session, err := manager.CreateSession(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != domain.SessionRunning || session.CreatedAt.IsZero() {
		t.Fatalf("unexpected created session: %#v", session)
	}
	if err := manager.Approve(context.Background(), session.SessionID, "early"); err == nil {
		t.Fatal("pre-approval must be rejected")
	}

	safeDecision, err := manager.EvaluateCommand(context.Background(), policy.CommandContext{
		SessionID: session.SessionID,
		Command:   "kubectl",
		Args:      []string{"get", "pods"},
	})
	if err != nil || safeDecision != policy.Approve {
		t.Fatalf("safe decision = %s, err=%v", safeDecision, err)
	}

	result := make(chan policy.Decision, 1)
	resultErr := make(chan error, 1)
	go func() {
		decision, evaluateErr := manager.EvaluateCommand(context.Background(), policy.CommandContext{
			SessionID: session.SessionID,
			AgentName: "mock",
			Command:   "kubectl",
			Args:      []string{"apply", "-f", "fixed.yaml"},
		})
		result <- decision
		resultErr <- evaluateErr
	}()

	waitForStatus(t, manager, session.SessionID, domain.SessionSuspended)
	if err := manager.Approve(context.Background(), session.SessionID, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := <-resultErr; err != nil {
		t.Fatal(err)
	}
	if decision := <-result; decision != policy.Approve {
		t.Fatalf("resumed decision = %s", decision)
	}
	waitForStatus(t, manager, session.SessionID, domain.SessionRunning)

	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.logs) != 3 {
		t.Fatalf("audit count = %d, want 3", len(audit.logs))
	}
	if audit.logs[2].Approver != "operator" || audit.logs[2].ExecutedCommand != "kubectl apply -f fixed.yaml" {
		t.Fatalf("approval audit = %#v", audit.logs[2])
	}
}

func TestHubReplaysEventsPublishedBeforeSubscription(t *testing.T) {
	hub := NewHub()
	event := EventEnvelope{Type: "reason.step", Data: mustJSON(map[string]int{"index": 1})}
	hub.Broadcast("session", event)
	subscription := hub.Subscribe("session")
	defer hub.Unsubscribe("session", subscription)

	select {
	case got := <-subscription:
		if got.Type != event.Type {
			t.Fatalf("event type = %q", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("history was not replayed")
	}
}

func TestSessionEventsArePersistedForReplay(t *testing.T) {
	sessions := newMemorySessions()
	audit := &memoryAudit{}
	events := &memoryEvents{}
	sandbox := &fakeRuntime{}
	manager := New(
		sessions,
		audit,
		sandbox,
		commandPolicy{},
		"sandbox:test",
		WithEventRepository(events),
	)
	session, err := manager.CreateSession(context.Background(), "event replay", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleHarnessEvent(context.Background(), session.SessionID, "reason.step", mustJSON(map[string]any{
		"index": 1,
		"title": "Inspect",
	})); err != nil {
		t.Fatal(err)
	}
	persisted, err := manager.ListEvents(context.Background(), session.SessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 2 || persisted[0].EventType != "session.started" || persisted[1].EventType != "reason.step" {
		t.Fatalf("persisted events = %#v", persisted)
	}
}

func TestManualTakeoverExecutesAndAuditsPolicy(t *testing.T) {
	manager, _, audit, _ := newTestManager()
	session, err := manager.CreateSession(context.Background(), "manual", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.ExecuteManualCommand(context.Background(), session.SessionID, []string{"kubectl", "get", "pods"}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "manual stdout" || result.PolicyDecision != domain.DecisionApprove {
		t.Fatalf("manual result = %#v", result)
	}
	_, err = manager.ExecuteManualCommand(context.Background(), session.SessionID, []string{"rm", "-rf", "/"}, "operator")
	if err == nil {
		t.Fatal("expected denied manual command")
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.logs) != 2 {
		t.Fatalf("audit logs = %#v", audit.logs)
	}
	if audit.logs[0].ExecutedCommand != "kubectl get pods" || audit.logs[1].PolicyDecision != domain.DecisionDeny {
		t.Fatalf("audit logs = %#v", audit.logs)
	}
}

func TestDestroyRemovesSandbox(t *testing.T) {
	manager, _, _, sandbox := newTestManager()
	session, err := manager.CreateSession(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Destroy(context.Background(), session.SessionID); err != nil {
		t.Fatal(err)
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if len(sandbox.destroyed) != 1 || sandbox.destroyed[0] != session.SessionID {
		t.Fatalf("destroyed = %v", sandbox.destroyed)
	}
	stored, err := manager.GetSession(context.Background(), session.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.SessionFailed {
		t.Fatalf("destroyed session status = %s", stored.Status)
	}
}

func TestDeniedToolResultCleansUpSandbox(t *testing.T) {
	manager, _, _, sandbox := newTestManager()
	session, err := manager.CreateSession(context.Background(), "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.HandleHarnessEvent(
		context.Background(),
		session.SessionID,
		"tool.result",
		mustJSON(map[string]string{"decision": "DENY"}),
	); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sandbox.mu.Lock()
		destroyed := len(sandbox.destroyed)
		sandbox.mu.Unlock()
		if destroyed == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("denied session sandbox was not cleaned up")
}

func waitForStatus(t *testing.T, manager *Manager, id string, want domain.SessionStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err := manager.GetSession(context.Background(), id)
		if err == nil && session.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach %s", id, want)
}
