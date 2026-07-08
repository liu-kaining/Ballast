package store

import (
	"context"
	"fmt"

	"github.com/ballast/ballast-server/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionEventStore persists the realtime session event stream for replay.
type SessionEventStore struct {
	pool *pgxpool.Pool
}

func (s *SessionEventStore) Append(ctx context.Context, event *domain.SessionEvent) (int64, error) {
	const q = `
		INSERT INTO ballast_session_events (session_id, event_type, event_data)
		VALUES ($1, $2, $3)
		RETURNING event_id`
	var id int64
	if err := s.pool.QueryRow(ctx, q, event.SessionID, event.EventType, event.EventData).Scan(&id); err != nil {
		return 0, fmt.Errorf("append session event: %w", err)
	}
	event.EventID = id
	return id, nil
}

func (s *SessionEventStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]*domain.SessionEvent, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	const q = `
		SELECT event_id, session_id, event_type, event_data, created_at
		FROM ballast_session_events
		WHERE session_id = $1
		ORDER BY event_id ASC
		LIMIT $2`
	rows, err := s.pool.Query(ctx, q, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	defer rows.Close()
	var out []*domain.SessionEvent
	for rows.Next() {
		var event domain.SessionEvent
		if err := rows.Scan(&event.EventID, &event.SessionID, &event.EventType, &event.EventData, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session event: %w", err)
		}
		out = append(out, &event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session events: %w", err)
	}
	return out, nil
}
