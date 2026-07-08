-- 7. 会话事件流表：持久化 Reason Tree / TTY / 策略事件，支持历史回放。
CREATE TABLE IF NOT EXISTS ballast_session_events (
    event_id   BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL REFERENCES ballast_sessions(session_id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    event_data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_session_events_session_event_id
    ON ballast_session_events(session_id, event_id);
