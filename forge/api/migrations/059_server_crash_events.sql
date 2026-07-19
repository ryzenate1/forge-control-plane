CREATE TABLE IF NOT EXISTS server_crash_events (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL,
    node_id TEXT NOT NULL,
    exit_code INTEGER NOT NULL DEFAULT 0,
    oom_killed BOOLEAN NOT NULL DEFAULT false,
    clean_exit BOOLEAN NOT NULL DEFAULT false,
    auto_restarted BOOLEAN NOT NULL DEFAULT false,
    crash_count INTEGER NOT NULL DEFAULT 1,
    node_state JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_crash_events_server_id ON server_crash_events (server_id);
CREATE INDEX IF NOT EXISTS idx_crash_events_created_at ON server_crash_events (created_at);
