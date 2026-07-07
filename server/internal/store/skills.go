package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ballast/ballast-server/internal/domain"
)

type SkillStore struct {
	pool *pgxpool.Pool
}

// Upsert 插入或更新一个 Skill（按 skill_id 冲突时覆盖）。
func (s *SkillStore) Upsert(ctx context.Context, sk *domain.Skill) error {
	const q = `
		INSERT INTO ballast_skills
			(skill_id, name, description, trigger_words, markdown_content, version, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (skill_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			trigger_words = EXCLUDED.trigger_words,
			markdown_content = EXCLUDED.markdown_content,
			version = EXCLUDED.version,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP`
	_, err := s.pool.Exec(ctx, q,
		sk.SkillID, sk.Name, sk.Description, sk.TriggerWords,
		sk.MarkdownContent, sk.Version, sk.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("upsert skill: %w", err)
	}
	return nil
}

// Get 按 ID 查询。
func (s *SkillStore) Get(ctx context.Context, id string) (*domain.Skill, error) {
	const q = `
			SELECT skill_id, name, COALESCE(description, ''), trigger_words, markdown_content, version, updated_by, updated_at
		FROM ballast_skills WHERE skill_id = $1`
	var sk domain.Skill
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&sk.SkillID, &sk.Name, &sk.Description, &sk.TriggerWords,
		&sk.MarkdownContent, &sk.Version, &sk.UpdatedBy, &sk.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	return &sk, nil
}

// List 列出所有 Skill。
func (s *SkillStore) List(ctx context.Context) ([]*domain.Skill, error) {
	const q = `
			SELECT skill_id, name, COALESCE(description, ''), trigger_words, markdown_content, version, updated_by, updated_at
		FROM ballast_skills ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	var out []*domain.Skill
	for rows.Next() {
		var sk domain.Skill
		if err := rows.Scan(
			&sk.SkillID, &sk.Name, &sk.Description, &sk.TriggerWords,
			&sk.MarkdownContent, &sk.Version, &sk.UpdatedBy, &sk.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, &sk)
	}
	return out, rows.Err()
}
