CREATE TABLE IF NOT EXISTS failover_policies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    node_id UUID NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    max_failures INTEGER NOT NULL DEFAULT 3,
    failure_window_sec INTEGER NOT NULL DEFAULT 300,
    cooldown_sec INTEGER NOT NULL DEFAULT 600,
    action TEXT NOT NULL DEFAULT 'notify',
    health_check_path TEXT NOT NULL DEFAULT '',
    health_check_port INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_failover_policies_node ON failover_policies(node_id);

CREATE TABLE IF NOT EXISTS failover_events (
    id UUID PRIMARY KEY,
    policy_id UUID REFERENCES failover_policies(id) ON DELETE SET NULL,
    node_id UUID NOT NULL,
    server_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'detected',
    message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_failover_events_node ON failover_events(node_id);
CREATE INDEX IF NOT EXISTS idx_failover_events_policy ON failover_events(policy_id);
