-- 2. 自动化路由规则表
CREATE TABLE IF NOT EXISTS ballast_trigger_rules (
    rule_id          VARCHAR(64) PRIMARY KEY,
    name             VARCHAR(128) NOT NULL,
    is_active        BOOLEAN DEFAULT TRUE,
    trigger_source   VARCHAR(64)  NOT NULL,                     -- e.g. 'Prometheus_Alertmanager'
    match_expression JSONB        NOT NULL,                     -- e.g. '{"alertname": "PodCrashLoop"}'
    bind_skills      VARCHAR(64)[] NOT NULL,                    -- 绑定的 Skill ID 数组
    agent_image      VARCHAR(255) NOT NULL,
    policy_group     VARCHAR(64)  NOT NULL                      -- 关联的 OPA 策略组
);
