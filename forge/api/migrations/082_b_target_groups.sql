CREATE TABLE IF NOT EXISTS target_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    algorithm TEXT NOT NULL DEFAULT 'round_robin',
    port INTEGER NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    health_check JSONB DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS target_group_targets (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES target_groups(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    ip TEXT NOT NULL,
    port INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'healthy' CHECK (status IN ('healthy', 'unhealthy', 'draining')),
    connections INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS target_group_targets_group_id_idx ON target_group_targets (group_id);
