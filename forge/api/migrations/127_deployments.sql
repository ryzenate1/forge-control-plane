CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL,
    strategy TEXT NOT NULL DEFAULT 'blue-green',
    status TEXT NOT NULL DEFAULT 'pending',
    image TEXT NOT NULL,
    blue_target_id TEXT NOT NULL DEFAULT '',
    green_target_id TEXT NOT NULL DEFAULT '',
    active_target TEXT NOT NULL DEFAULT 'blue',
    health_check_path TEXT NOT NULL DEFAULT '',
    health_check_port INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deployments_server ON deployments(server_id);
