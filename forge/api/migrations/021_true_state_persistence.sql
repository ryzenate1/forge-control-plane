DO $$ BEGIN
    BEGIN
        CREATE TYPE server_desired_state AS ENUM ('running', 'stopped');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE server_actual_state AS ENUM ('running', 'stopped', 'starting', 'stopping', 'installing', 'crashed', 'unknown');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE node_desired_state AS ENUM ('active', 'maintenance', 'draining');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE node_actual_state AS ENUM ('online', 'offline', 'degraded');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS desired_state server_desired_state NOT NULL DEFAULT 'stopped',
    ADD COLUMN IF NOT EXISTS actual_state server_actual_state NOT NULL DEFAULT 'unknown';

UPDATE servers
SET desired_state = CASE
        WHEN status = 'running' THEN 'running'::server_desired_state
        ELSE 'stopped'::server_desired_state
    END,
    actual_state = CASE
        WHEN status = 'running' THEN 'running'::server_actual_state
        WHEN status = 'stopped' THEN 'stopped'::server_actual_state
        WHEN status = 'installing' THEN 'installing'::server_actual_state
        WHEN status = 'install_failed' THEN 'crashed'::server_actual_state
        ELSE 'unknown'::server_actual_state
    END;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS desired_state node_desired_state NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS actual_state node_actual_state NOT NULL DEFAULT 'offline';

UPDATE nodes
SET desired_state = CASE
        WHEN maintenance_mode THEN 'maintenance'::node_desired_state
        WHEN COALESCE(draining, false) THEN 'draining'::node_desired_state
        ELSE 'active'::node_desired_state
    END,
    actual_state = CASE
        WHEN status = 'online' THEN 'online'::node_actual_state
        WHEN status = 'degraded' THEN 'degraded'::node_actual_state
        ELSE 'offline'::node_actual_state
    END;

CREATE TABLE IF NOT EXISTS state_transitions (
    id UUID PRIMARY KEY,
    resource_type TEXT NOT NULL,
    resource_id UUID NOT NULL,
    state_kind TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS state_transitions_resource_idx ON state_transitions (resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS state_transitions_created_at_idx ON state_transitions (created_at DESC);
CREATE INDEX IF NOT EXISTS servers_desired_actual_state_idx ON servers (desired_state, actual_state);
CREATE INDEX IF NOT EXISTS nodes_desired_actual_state_idx ON nodes (desired_state, actual_state);
