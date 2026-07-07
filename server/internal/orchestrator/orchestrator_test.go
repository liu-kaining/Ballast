package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

type memorySkills map[string]*domain.Skill

func (m memorySkills) Get(_ context.Context, id string) (*domain.Skill, error) {
	skill := m[id]
	if skill == nil {
		return nil, errors.New("not found")
	}
	cloned := *skill
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
	return nil, nil, nil
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
