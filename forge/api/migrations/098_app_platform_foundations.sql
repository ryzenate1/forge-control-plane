-- App-platform tables must be in the runtime migration directory.  This
-- migration intentionally sorts before 095_team_tenancy.sql, which adds
-- organization references to domains, git sources, and compose projects.

CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    domain TEXT NOT NULL UNIQUE,
    wildcard BOOLEAN NOT NULL DEFAULT FALSE,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    verification_token TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS domains_server_id_idx ON domains (server_id);
CREATE INDEX IF NOT EXISTS domains_verified_idx ON domains (verified);

CREATE TABLE IF NOT EXISTS compose_projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    server_id TEXT,
    compose_content TEXT NOT NULL DEFAULT '',
    parsed_config JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'imported',
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS compose_stacks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    node_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'deploying',
    compose_yaml TEXT NOT NULL DEFAULT '',
    compose_hash TEXT NOT NULL DEFAULT '',
    env_vars JSONB NOT NULL DEFAULT '{}',
    memory_mb BIGINT NOT NULL DEFAULT 0,
    cpu_shares BIGINT NOT NULL DEFAULT 0,
    disk_mb BIGINT NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    reservation_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compose_stacks_user_idx ON compose_stacks (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS compose_stacks_node_idx ON compose_stacks (node_id);

CREATE TABLE IF NOT EXISTS db_containers (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    version TEXT NOT NULL,
    container_id TEXT NOT NULL DEFAULT '',
    connection_string TEXT NOT NULL DEFAULT '',
    credentials JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    port INTEGER NOT NULL DEFAULT 0,
    volume_id TEXT NOT NULL DEFAULT '',
    memory_mb INTEGER NOT NULL DEFAULT 256,
    cpu_shares INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS db_containers_server_idx ON db_containers (server_id, created_at DESC);

CREATE TABLE IF NOT EXISTS builds (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    builder_type TEXT NOT NULL CHECK (builder_type IN ('dockerfile', 'nixpacks')),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed', 'canceled', 'abandoned')),
    image_ref TEXT,
    build_log TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    exit_code INTEGER,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS builds_source_idx ON builds (source_id, started_at DESC);
