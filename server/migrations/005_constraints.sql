ALTER TABLE ballast_sessions
    ADD CONSTRAINT ballast_sessions_trigger_type_check
    CHECK (trigger_type IN ('WEBHOOK', 'CRON', 'MANUAL_CHAT'));

ALTER TABLE ballast_sessions
    ADD CONSTRAINT ballast_sessions_status_check
    CHECK (status IN ('RUNNING', 'SUSPENDED', 'SUCCESS', 'FAILED'));

ALTER TABLE ballast_audit_logs
    ADD CONSTRAINT ballast_audit_logs_session_fk
    FOREIGN KEY (session_id) REFERENCES ballast_sessions(session_id);

ALTER TABLE ballast_audit_logs
    ADD CONSTRAINT ballast_audit_logs_policy_decision_check
    CHECK (policy_decision IS NULL OR policy_decision IN ('APPROVE', 'SUSPEND', 'DENY'));
