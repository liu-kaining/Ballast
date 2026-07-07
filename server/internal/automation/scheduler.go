// Package automation contains trigger execution plumbing.
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/ballast/ballast-server/internal/orchestrator"
)

type TriggerRuleRepository interface {
	List(context.Context) ([]*domain.TriggerRule, error)
}

type SessionCreator interface {
	CreateSessionWithOptions(context.Context, orchestrator.CreateSessionOptions) (*domain.Session, error)
}

type Scheduler struct {
	rules   TriggerRuleRepository
	creator SessionCreator
	logger  *log.Logger
	every   time.Duration

	mu      sync.Mutex
	lastRun map[string]time.Time
}

func NewScheduler(rules TriggerRuleRepository, creator SessionCreator, logger *log.Logger, every time.Duration) *Scheduler {
	if every <= 0 {
		every = 30 * time.Second
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		rules:   rules,
		creator: creator,
		logger:  logger,
		every:   every,
		lastRun: make(map[string]time.Time),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.every)
	defer ticker.Stop()
	_ = s.Tick(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.Tick(ctx, now.UTC()); err != nil {
				s.logger.Printf("cron scheduler tick: %v", err)
			}
		}
	}
}

func (s *Scheduler) Tick(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rules, err := s.rules.List(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.IsActive || !strings.EqualFold(rule.TriggerSource, "cron") {
			continue
		}
		spec, err := parseCronSpec(rule.MatchExpression)
		if err != nil {
			s.logger.Printf("cron rule %s ignored: %v", rule.RuleID, err)
			continue
		}
		if spec.Interval <= 0 {
			continue
		}
		if last := s.lastRun[rule.RuleID]; !last.IsZero() && now.Sub(last) < spec.Interval {
			continue
		}
		title := strings.TrimSpace(spec.Title)
		if title == "" {
			title = rule.Name
		}
		if _, err := s.creator.CreateSessionWithOptions(ctx, orchestrator.CreateSessionOptions{
			Title:       title,
			AgentImage:  rule.AgentImage,
			TriggerType: domain.TriggerCron,
			SkillIDs:    rule.BindSkills,
		}); err != nil {
			return fmt.Errorf("create cron session for rule %s: %w", rule.RuleID, err)
		}
		s.lastRun[rule.RuleID] = now
	}
	return nil
}

type cronSpec struct {
	Interval time.Duration
	Title    string
}

func parseCronSpec(raw json.RawMessage) (cronSpec, error) {
	var body struct {
		IntervalSeconds int    `json:"interval_seconds"`
		CronExpression  string `json:"cron_expression"`
		Title           string `json:"title"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return cronSpec{}, err
	}
	var interval time.Duration
	if body.IntervalSeconds > 0 {
		interval = time.Duration(body.IntervalSeconds) * time.Second
	}
	if interval == 0 && strings.HasPrefix(strings.TrimSpace(body.CronExpression), "@every ") {
		durationText := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body.CronExpression), "@every "))
		parsed, err := time.ParseDuration(durationText)
		if err != nil {
			return cronSpec{}, err
		}
		interval = parsed
	}
	if interval <= 0 {
		return cronSpec{}, fmt.Errorf("cron match_expression needs interval_seconds or cron_expression=@every <duration>")
	}
	if interval < time.Second {
		return cronSpec{}, fmt.Errorf("cron interval must be at least one second")
	}
	return cronSpec{Interval: interval, Title: body.Title}, nil
}
