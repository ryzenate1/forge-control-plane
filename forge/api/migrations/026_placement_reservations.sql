DO $$ BEGIN
    BEGIN
        CREATE TYPE placement_reservation_type AS ENUM ('placement', 'migration', 'recovery', 'evacuation');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE placement_reservation_status AS ENUM ('pending', 'active', 'completed', 'expired', 'cancelled');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS placement_reservations (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    server_id uuid REFERENCES servers(id) ON DELETE SET NULL,
    migration_id uuid REFERENCES migrations(id) ON DELETE SET NULL,
    reservation_type placement_reservation_type NOT NULL,
    cpu integer NOT NULL DEFAULT 0,
    memory integer NOT NULL DEFAULT 0,
    disk integer NOT NULL DEFAULT 0,
    status placement_reservation_status NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz,
    cancelled_at timestamptz,
    expired_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_placement_reservations_node_active
    ON placement_reservations (node_id, status, expires_at);

CREATE INDEX IF NOT EXISTS idx_placement_reservations_server_active
    ON placement_reservations (server_id, status)
    WHERE server_id IS NOT NULL AND status IN ('pending', 'active');

CREATE INDEX IF NOT EXISTS idx_placement_reservations_migration_active
    ON placement_reservations (migration_id, status)
    WHERE migration_id IS NOT NULL AND status IN ('pending', 'active');

CREATE INDEX IF NOT EXISTS idx_placement_reservations_expires
    ON placement_reservations (expires_at, status);
