CREATE TABLE IF NOT EXISTS git_deployments (
    id              TEXT PRIMARY KEY,
    git_source_id   TEXT NOT NULL REFERENCES git_sources(id) ON DELETE CASCADE,
    commit_sha      TEXT NOT NULL DEFAULT '',
    branch          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',
    status_message  TEXT NOT NULL DEFAULT '',
    image_tag       TEXT NOT NULL DEFAULT '',
    build_log       TEXT NOT NULL DEFAULT '',
    deploy_log      TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_deployments_source_idx ON git_deployments (git_source_id);
CREATE INDEX IF NOT EXISTS git_deployments_status_idx ON git_deployments (status);

CREATE TABLE IF NOT EXISTS git_deployment_hooks (
    id            TEXT PRIMARY KEY,
    git_source_id TEXT NOT NULL REFERENCES git_sources(id) ON DELETE CASCADE,
    secret        TEXT NOT NULL DEFAULT '',
    events        TEXT[] NOT NULL DEFAULT '{"push"}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_deployment_hooks_source_idx ON git_deployment_hooks (git_source_id);
