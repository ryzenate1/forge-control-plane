CREATE TABLE IF NOT EXISTS scaling_policies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    server_id UUID NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    min_memory_mb BIGINT NOT NULL DEFAULT 512,
    max_memory_mb BIGINT NOT NULL DEFAULT 4096,
    min_cpu BIGINT NOT NULL DEFAULT 100,
    max_cpu BIGINT NOT NULL DEFAULT 400,
    target_cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0.70,
    target_memory_percent DOUBLE PRECISION NOT NULL DEFAULT 0.70,
    scale_up_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.80,
    scale_down_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.30,
    cooldown_seconds INTEGER NOT NULL DEFAULT 120,
    poll_interval_seconds INTEGER NOT NULL DEFAULT 30,
    scale_up_factor DOUBLE PRECISION NOT NULL DEFAULT 1.25,
    scale_down_factor DOUBLE PRECISION NOT NULL DEFAULT 0.75,
    max_scale_up_step_mb BIGINT NOT NULL DEFAULT 0,
    max_scale_down_step_mb BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scaling_policies_server ON scaling_policies(server_id);

CREATE TABLE IF NOT EXISTS scaling_events (
    id UUID PRIMARY KEY,
    policy_id UUID REFERENCES scaling_policies(id) ON DELETE CASCADE,
    server_id UUID NOT NULL,
    direction TEXT NOT NULL,
    old_memory BIGINT NOT NULL DEFAULT 0,
    new_memory BIGINT NOT NULL DEFAULT 0,
    old_cpu BIGINT NOT NULL DEFAULT 0,
    new_cpu BIGINT NOT NULL DEFAULT 0,
    cpu_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
    mem_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason TEXT NOT NULL DEFAULT '',
    success BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scaling_events_policy ON scaling_events(policy_id);
