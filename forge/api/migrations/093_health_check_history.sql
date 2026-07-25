CREATE TABLE IF NOT EXISTS health_check_history (
    id BIGSERIAL PRIMARY KEY,
    target_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    server_id TEXT NOT NULL DEFAULT '',
    check_type TEXT NOT NULL DEFAULT 'tcp',
    status TEXT NOT NULL CHECK (status IN ('healthy', 'unhealthy', 'suspected')),
    latency_ms INTEGER NOT NULL DEFAULT 0,
    status_code INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS health_check_history_target_id_idx ON health_check_history (target_id);
CREATE INDEX IF NOT EXISTS health_check_history_group_id_idx ON health_check_history (group_id);
CREATE INDEX IF NOT EXISTS health_check_history_checked_at_idx ON health_check_history (checked_at DESC);
