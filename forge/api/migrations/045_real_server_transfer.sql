CREATE TABLE IF NOT EXISTS migration_runs (
    migration_id UUID PRIMARY KEY REFERENCES migrations(id) ON DELETE CASCADE,
    protocol_version TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT 'planned',
    idempotency_key UUID NOT NULL UNIQUE,
    attempt INTEGER NOT NULL DEFAULT 0,
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    target_allocation_id UUID REFERENCES allocations(id),
    archive_size BIGINT NOT NULL DEFAULT 0,
    archive_checksum TEXT NOT NULL DEFAULT '',
    source_credential_hash TEXT NOT NULL DEFAULT '',
    destination_credential_hash TEXT NOT NULL DEFAULT '',
    credential_expires_at TIMESTAMPTZ,
    cleanup_pending BOOLEAN NOT NULL DEFAULT FALSE,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS migration_allocation_reservations (
    allocation_id UUID PRIMARY KEY REFERENCES allocations(id) ON DELETE CASCADE,
    migration_id UUID NOT NULL UNIQUE REFERENCES migrations(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS migration_runs_reclaim_idx
    ON migration_runs (phase, lease_expires_at)
    WHERE phase NOT IN ('completed', 'failed', 'cancelled');
