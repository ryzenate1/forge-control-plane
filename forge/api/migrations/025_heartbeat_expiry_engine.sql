DO $$ BEGIN
    BEGIN
        CREATE TYPE node_heartbeat_state AS ENUM ('healthy', 'suspected', 'unreachable', 'offline', 'recovering');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS heartbeat_state node_heartbeat_state NOT NULL DEFAULT 'offline',
    ADD COLUMN IF NOT EXISTS heartbeat_state_changed_at timestamptz,
    ADD COLUMN IF NOT EXISTS heartbeat_recovery_count integer NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_nodes_heartbeat_state ON nodes (heartbeat_state);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen_at ON nodes (last_seen_at);
