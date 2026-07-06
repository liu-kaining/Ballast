-- 1. 会话主表 (管理任务与 Chat 全局生命周期)
CREATE TABLE IF NOT EXISTS ballast_sessions (
    session_id   VARCHAR(64) PRIMARY KEY,
    title        VARCHAR(255) NOT NULL,
    trigger_type VARCHAR(32)  NOT NULL,                       -- 'WEBHOOK', 'CRON', 'MANUAL_CHAT'
    status       VARCHAR(32)  NOT NULL,                       -- 'RUNNING', 'SUSPENDED', 'SUCCESS', 'FAILED'
    agent_image  VARCHAR(255) NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session_status ON ballast_sessions(status);
