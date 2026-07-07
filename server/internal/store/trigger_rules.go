package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ballast/ballast-server/internal/domain"
)

type TriggerRuleStore struct {
	pool *pgxpool.Pool
}

// Upsert 插入或更新一条自动化触发路由规则。
func (s *TriggerRuleStore) Upsert(ctx context.Context, rule *domain.TriggerRule) error {
	const q = `
		INSERT INTO ballast_trigger_rules
			(rule_id, name, is_active, trigger_source, match_expression, bind_skills, agent_image, policy_group)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8)
		ON CONFLICT (rule_id) DO UPDATE SET
			name = EXCLUDED.name,
			is_active = EXCLUDED.is_active,
			trigger_source = EXCLUDED.trigger_source,
			match_expression = EXCLUDED.match_expression,
			bind_skills = EXCLUDED.bind_skills,
			agent_image = EXCLUDED.agent_image,
			policy_group = EXCLUDED.policy_group`
	_, err := s.pool.Exec(ctx, q,
		rule.RuleID, rule.Name, rule.IsActive, rule.TriggerSource,
		string(rule.MatchExpression), rule.BindSkills, rule.AgentImage, rule.PolicyGroup,
	)
	if err != nil {
		return fmt.Errorf("upsert trigger rule: %w", err)
	}
	return nil
}

// Get 按 ID 查询一条触发路由规则。
func (s *TriggerRuleStore) Get(ctx context.Context, id string) (*domain.TriggerRule, error) {
	const q = `
		SELECT rule_id, name, is_active, trigger_source, match_expression, bind_skills, agent_image, policy_group
		FROM ballast_trigger_rules WHERE rule_id = $1`
	var rule domain.TriggerRule
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&rule.RuleID, &rule.Name, &rule.IsActive, &rule.TriggerSource,
		&rule.MatchExpression, &rule.BindSkills, &rule.AgentImage, &rule.PolicyGroup,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get trigger rule: %w", err)
	}
	return &rule, nil
}

// List 列出所有触发路由规则。
func (s *TriggerRuleStore) List(ctx context.Context) ([]*domain.TriggerRule, error) {
	const q = `
		SELECT rule_id, name, is_active, trigger_source, match_expression, bind_skills, agent_image, policy_group
		FROM ballast_trigger_rules ORDER BY name ASC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list trigger rules: %w", err)
	}
	defer rows.Close()
	var out []*domain.TriggerRule
	for rows.Next() {
		var rule domain.TriggerRule
		if err := rows.Scan(
			&rule.RuleID, &rule.Name, &rule.IsActive, &rule.TriggerSource,
			&rule.MatchExpression, &rule.BindSkills, &rule.AgentImage, &rule.PolicyGroup,
		); err != nil {
			return nil, fmt.Errorf("scan trigger rule: %w", err)
		}
		out = append(out, &rule)
	}
	return out, rows.Err()
}
