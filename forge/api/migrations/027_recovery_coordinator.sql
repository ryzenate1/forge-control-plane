DO $$ BEGIN
    BEGIN
        CREATE TYPE recovery_plan_status AS ENUM ('pending', 'planning', 'planned', 'failed', 'cancelled');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE recovery_item_status AS ENUM ('pending', 'planned', 'failed', 'cancelled');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS recovery_plans (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    status recovery_plan_status NOT NULL DEFAULT 'pending',
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recovery_items (
    id uuid PRIMARY KEY,
    plan_id uuid NOT NULL REFERENCES recovery_plans(id) ON DELETE CASCADE,
    server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    source_node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_node_id uuid REFERENCES nodes(id) ON DELETE SET NULL,
    reservation_id uuid REFERENCES placement_reservations(id) ON DELETE SET NULL,
    migration_id uuid REFERENCES migrations(id) ON DELETE SET NULL,
    status recovery_item_status NOT NULL DEFAULT 'pending',
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_recovery_plans_node_status ON recovery_plans (node_id, status);
CREATE INDEX IF NOT EXISTS idx_recovery_items_plan ON recovery_items (plan_id);
CREATE INDEX IF NOT EXISTS idx_recovery_items_server ON recovery_items (server_id);
CREATE INDEX IF NOT EXISTS idx_recovery_items_migration ON recovery_items (migration_id);
