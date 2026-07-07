package automation

import (
	"context"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/orchestrator"
)

type memoryRuleRepo struct {
	rules []*domain.TriggerRule
}

func (m memoryRuleRepo) List(context.Context) ([]*domain.TriggerRule, error) {
	return m.rules, nil
}

type recordingCreator struct {
	options []orchestrator.CreateSessionOptions
}

func (r *recordingCreator) CreateSessionWithOptions(_ context.Context, options orchestrator.CreateSessionOptions) (*domain.Session, error) {
	r.options = append(r.options, options)
	return &domain.Session{SessionID: "sess-test"}, nil
}

func TestSchedulerCreatesDueCronSession(t *testing.T) {
	creator := &recordingCreator{}
	scheduler := NewScheduler(memoryRuleRepo{rules: []*domain.TriggerRule{
		{
			RuleID:          "drift",
			Name:            "Drift check",
			IsActive:        true,
			TriggerSource:   "cron",
			MatchExpression: json.RawMessage(`{"interval_seconds": 60, "title": "Daily drift"}`),
			BindSkills:      []string{"tf-drift"},
			AgentImage:      "ballast-runner-base:dev",
		},
	}}, creator, log.Default(), time.Second)

	now := time.Unix(1_700_000_000, 0)
	if err := scheduler.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Tick(context.Background(), now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(creator.options) != 1 {
		t.Fatalf("created sessions = %d", len(creator.options))
	}
	got := creator.options[0]
	if got.TriggerType != domain.TriggerCron || got.Title != "Daily drift" || got.SkillIDs[0] != "tf-drift" {
		t.Fatalf("options = %#v", got)
	}
}

func TestParseCronEveryExpression(t *testing.T) {
	spec, err := parseCronSpec(json.RawMessage(`{"cron_expression":"@every 5m"}`))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Interval != 5*time.Minute {
		t.Fatalf("interval = %s", spec.Interval)
	}
}
