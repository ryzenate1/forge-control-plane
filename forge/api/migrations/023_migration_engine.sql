DO $$ BEGIN
    BEGIN
        CREATE TYPE migration_status AS ENUM (
            'planned',
            'preparing',
            'transferring',
            'restoring',
            'completed',
            'failed',
            'cancelled'
        );
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS migrations (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    source_node_id UUID NOT NULL REFERENCES nodes(id),
    target_node_id UUID NOT NULL REFERENCES nodes(id),
    status migration_status NOT NULL DEFAULT 'planned',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS migration_history (
    id UUID PRIMARY KEY,
    migration_id UUID NOT NULL REFERENCES migrations(id) ON DELETE CASCADE,
    from_status migration_status,
    to_status migration_status NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS migrations_server_idx ON migrations (server_id);
CREATE INDEX IF NOT EXISTS migrations_source_node_idx ON migrations (source_node_id);
CREATE INDEX IF NOT EXISTS migrations_target_node_idx ON migrations (target_node_id);
CREATE INDEX IF NOT EXISTS migrations_status_idx ON migrations (status);
CREATE INDEX IF NOT EXISTS migration_history_migration_idx ON migration_history (migration_id, created_at);
