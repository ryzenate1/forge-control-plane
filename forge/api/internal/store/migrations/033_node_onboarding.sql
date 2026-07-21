-- Node enrollment and capability synchronization
-- Inspired by Komodo Core/Periphery onboarding key patterns

CREATE TABLE IF NOT EXISTS onboarding_tokens (
    id UUID PRIMARY KEY,
    token_hash TEXT NOT NULL,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    approved_by TEXT,
    revoked_at TIMESTAMPTZ,
    revoked_reason TEXT,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'revoked', 'expired')),
    UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS onboarding_tokens_node_id_idx ON onboarding_tokens (node_id);
CREATE INDEX IF NOT EXISTS onboarding_tokens_state_idx ON onboarding_tokens (state);

CREATE TABLE IF NOT EXISTS node_capabilities (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    beacon_version TEXT NOT NULL DEFAULT '',
    os TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    cpu_threads INTEGER NOT NULL DEFAULT 0,
    memory_mb BIGINT NOT NULL DEFAULT 0,
    disk_mb BIGINT NOT NULL DEFAULT 0,
    uptime_seconds BIGINT NOT NULL DEFAULT 0,

    -- Capability flags
    runtime_available BOOLEAN NOT NULL DEFAULT FALSE,
    runtime_status TEXT NOT NULL DEFAULT '',
    runtime_version TEXT NOT NULL DEFAULT '',
    runtime_provider TEXT NOT NULL DEFAULT '',

    docker_build_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    nixpacks_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    compose_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    compose_version TEXT NOT NULL DEFAULT '',
    compose_stack_count INTEGER NOT NULL DEFAULT 0,

    local_backups BOOLEAN NOT NULL DEFAULT FALSE,
    s3_backups BOOLEAN NOT NULL DEFAULT FALSE,
    transfer_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    sftp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    websocket_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    console_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    database_provisioning_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    -- Full JSON capability report for extensibility
    raw_report JSONB NOT NULL DEFAULT '{}',

    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (node_id)
);

CREATE INDEX IF NOT EXISTS node_capabilities_fetched_at_idx ON node_capabilities (fetched_at DESC);

-- Node capability history for tracking changes over time
CREATE TABLE IF NOT EXISTS node_capability_history (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    beacon_version TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]',
    raw_report JSONB NOT NULL DEFAULT '{}',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS node_capability_history_node_id_idx ON node_capability_history (node_id, observed_at DESC);
