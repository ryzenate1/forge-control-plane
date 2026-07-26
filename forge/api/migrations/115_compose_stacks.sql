-- WP2: Docker Compose Support -- extended compose stack model
-- Adds compose_type, source_type, environment_id to compose_stacks
-- Creates compose_services and compose_logs tables

ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS compose_type TEXT NOT NULL DEFAULT 'docker-compose';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'raw';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS environment_id TEXT;

CREATE TABLE IF NOT EXISTS compose_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stack_id UUID NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    image TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    state TEXT NOT NULL DEFAULT '',
    ports TEXT NOT NULL DEFAULT '',
    health TEXT NOT NULL DEFAULT '',
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compose_services_stack_idx ON compose_services (stack_id);
CREATE INDEX IF NOT EXISTS compose_services_node_idx ON compose_services (node_id);

CREATE TABLE IF NOT EXISTS compose_logs (
    id BIGSERIAL PRIMARY KEY,
    stack_id UUID NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    service_name TEXT NOT NULL DEFAULT '',
    stream TEXT NOT NULL DEFAULT 'stdout',
    message TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compose_logs_stack_idx ON compose_logs (stack_id, timestamp DESC);

CREATE INDEX IF NOT EXISTS compose_stacks_env_idx ON compose_stacks (environment_id) WHERE environment_id IS NOT NULL;

DO $$ BEGIN ALTER TABLE compose_projects ADD CONSTRAINT fk_compose_projects_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE compose_stacks ADD CONSTRAINT fk_compose_stacks_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE compose_stacks ADD CONSTRAINT fk_compose_stacks_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE compose_stacks ADD CONSTRAINT fk_compose_stacks_reservation FOREIGN KEY (reservation_id) REFERENCES placement_reservations(id) ON DELETE SET NULL; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE compose_services ADD CONSTRAINT fk_compose_services_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
