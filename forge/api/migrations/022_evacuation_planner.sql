DO $$ BEGIN
    BEGIN
        CREATE TYPE evacuation_plan_status AS ENUM ('pending', 'running', 'completed', 'failed');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS evacuation_plans (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    status evacuation_plan_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS evacuation_items (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL REFERENCES evacuation_plans(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    source_node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    eligible BOOLEAN NOT NULL DEFAULT FALSE,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS evacuation_plans_node_idx ON evacuation_plans (node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS evacuation_items_plan_idx ON evacuation_items (plan_id);
CREATE INDEX IF NOT EXISTS evacuation_items_server_idx ON evacuation_items (server_id);
