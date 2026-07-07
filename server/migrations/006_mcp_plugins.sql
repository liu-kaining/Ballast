-- 6. MCP 插件资产表
CREATE TABLE IF NOT EXISTS ballast_mcp_plugins (
    plugin_id  VARCHAR(64) PRIMARY KEY,
    name       VARCHAR(128) NOT NULL,
    command    TEXT NOT NULL,
    args       TEXT[] NOT NULL DEFAULT '{}',
    env        JSONB NOT NULL DEFAULT '{}',
    is_active  BOOLEAN DEFAULT TRUE,
    updated_by VARCHAR(64) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
