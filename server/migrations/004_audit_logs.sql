-- 4. 全量 TTY 变更审计日志表
-- 生产环境高并发写入建议挂载到 ClickHouse，此处展示核心结构。
CREATE TABLE IF NOT EXISTS ballast_audit_logs (
    audit_id          BIGSERIAL PRIMARY KEY,
    session_id        VARCHAR(64) NOT NULL,
    loop_index        INT NOT NULL,                             -- OpenCode 第几次思考循环
    model_name        VARCHAR(64),
    prompt_tokens     INT DEFAULT 0,
    completion_tokens INT DEFAULT 0,
    executed_command  TEXT,                                     -- 拦截或执行的命令
    policy_decision   VARCHAR(32),                              -- 'APPROVE', 'SUSPEND', 'DENY'
    approver          VARCHAR(64),                              -- 放行审批人
    raw_tty_output_path TEXT,                                   -- 指向对象存储的本轮 stdout/stderr 原始流路径
    created_at        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_session_id ON ballast_audit_logs(session_id);
