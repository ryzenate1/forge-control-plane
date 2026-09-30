CREATE TABLE IF NOT EXISTS git_deployment_hooks (
    id TEXT PRIMARY KEY,
    git_source_id TEXT NOT NULL REFERENCES git_sources(id) ON DELETE CASCADE,
    secret TEXT NOT NULL,
    events TEXT NOT NULL DEFAULT '["push"]',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS git_deployment_hooks_source_idx
    ON git_deployment_hooks (git_source_id);
