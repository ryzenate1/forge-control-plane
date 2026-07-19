-- Add missing foreign key constraints, fix type mismatch, and add missing index.
-- Uses DO blocks for idempotency across supported PostgreSQL versions.

-- 1) server_crash_events: fix node_id type from TEXT to UUID to match nodes(id)
ALTER TABLE server_crash_events ALTER COLUMN node_id TYPE UUID USING node_id::uuid;

-- 2) server_crash_events: add FK on server_id -> servers(id)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_crash_events_server') THEN
        ALTER TABLE server_crash_events
            ADD CONSTRAINT fk_crash_events_server
            FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 3) server_crash_events: add FK on node_id -> nodes(id)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_crash_events_node') THEN
        ALTER TABLE server_crash_events
            ADD CONSTRAINT fk_crash_events_node
            FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL;
    END IF;
END $$;

-- 4) server_crash_events: add missing index on node_id
CREATE INDEX IF NOT EXISTS idx_crash_events_node_id ON server_crash_events (node_id);

-- 5) deployments: add FK on server_id -> servers(id)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_deployments_server') THEN
        ALTER TABLE deployments
            ADD CONSTRAINT fk_deployments_server
            FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 6) failover_policies: add FK on node_id -> nodes(id)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_failover_policies_node') THEN
        ALTER TABLE failover_policies
            ADD CONSTRAINT fk_failover_policies_node
            FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;
    END IF;
END $$;
