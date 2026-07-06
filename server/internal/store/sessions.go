package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ballast/ballast-server/internal/domain"
)

type SessionStore struct {
	pool *pgxpool.Pool
}

// ErrNotFound 表示查询无结果。
var ErrNotFound = errors.New("store: not found")

// Create 插入一条会话。
func (s *SessionStore) Create(ctx context.Context, sess *domain.Session) error {
	const q = `
		INSERT INTO ballast_sessions (session_id, title, trigger_type, status, agent_image)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := s.pool.Exec(ctx, q,
		sess.SessionID, sess.Title, sess.TriggerType, sess.Status, sess.AgentImage,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// Get 按 ID 查询单条会话。
func (s *SessionStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	const q = `
		SELECT session_id, title, trigger_type, status, agent_image, created_at, updated_at
		FROM ballast_sessions WHERE session_id = $1`
	var sess domain.Session
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&sess.SessionID, &sess.Title, &sess.TriggerType, &sess.Status,
		&sess.AgentImage, &sess.CreatedAt, &sess.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

// List 按 status 过滤列出会话，limit/offset 分页。
func (s *SessionStore) List(ctx context.Context, status domain.SessionStatus, limit, offset int) ([]*domain.Session, error) {
	const q = `
		SELECT session_id, title, trigger_type, status, agent_image, created_at, updated_at
		FROM ballast_sessions
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := s.pool.Query(ctx, q, string(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []*domain.Session
	for rows.Next() {
		var sess domain.Session
		if err := rows.Scan(
			&sess.SessionID, &sess.Title, &sess.TriggerType, &sess.Status,
			&sess.AgentImage, &sess.CreatedAt, &sess.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, &sess)
	}
	return out, rows.Err()
}

// UpdateStatus 更新会话状态。
func (s *SessionStore) UpdateStatus(ctx context.Context, id string, status domain.SessionStatus) error {
	const q = `UPDATE ballast_sessions SET status = $2, updated_at = CURRENT_TIMESTAMP WHERE session_id = $1`
	ct, err := s.pool.Exec(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
