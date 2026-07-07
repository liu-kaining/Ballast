package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ballast/ballast-server/internal/domain"
)

type AuditStore struct {
	pool *pgxpool.Pool
}

// Append 写入一条审计日志，返回生成的 audit_id。
func (s *AuditStore) Append(ctx context.Context, log *domain.AuditLog) (int64, error) {
	const q = `
		INSERT INTO ballast_audit_logs
			(session_id, loop_index, model_name, prompt_tokens, completion_tokens,
			 executed_command, policy_decision, approver, raw_tty_output_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING audit_id`
	var id int64
	err := s.pool.QueryRow(ctx, q,
		log.SessionID, log.LoopIndex, log.ModelName, log.PromptTokens, log.CompletionTokens,
		log.ExecutedCommand, log.PolicyDecision, log.Approver, log.RawTTYOutputPath,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert audit log: %w", err)
	}
	return id, nil
}

// ListBySession 按 session 拉取审计日志。
func (s *AuditStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]*domain.AuditLog, error) {
	const q = `
			SELECT audit_id, session_id, loop_index, COALESCE(model_name, ''), prompt_tokens, completion_tokens,
			       COALESCE(executed_command, ''), COALESCE(policy_decision, ''),
			       COALESCE(approver, ''), COALESCE(raw_tty_output_path, ''), created_at
		FROM ballast_audit_logs
		WHERE session_id = $1
		ORDER BY audit_id ASC
		LIMIT $2`
	rows, err := s.pool.Query(ctx, q, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()
	var out []*domain.AuditLog
	for rows.Next() {
		var l domain.AuditLog
		if err := rows.Scan(
			&l.AuditID, &l.SessionID, &l.LoopIndex, &l.ModelName, &l.PromptTokens,
			&l.CompletionTokens, &l.ExecutedCommand, &l.PolicyDecision, &l.Approver,
			&l.RawTTYOutputPath, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}
