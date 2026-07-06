-- 3. SRE 剧本 Skill 资产表
CREATE TABLE IF NOT EXISTS ballast_skills (
    skill_id         VARCHAR(64) PRIMARY KEY,
    name             VARCHAR(128) NOT NULL,
    description      TEXT,
    trigger_words    TEXT[] NOT NULL,
    markdown_content TEXT NOT NULL,                             -- 包含 Frontmatter 的标准 OpenCode SKILL.md
    version          INT DEFAULT 1,
    updated_by       VARCHAR(64) NOT NULL,
    updated_at       TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
