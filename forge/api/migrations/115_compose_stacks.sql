-- WP2: Docker Compose Support — extended compose stack model
-- Adds compose_type, source_type, environment_id to compose_stacks
-- Creates compose_services and compose_logs tables

ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS compose_type TEXT NOT NULL DEFAULT 'docker-compose';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'raw';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS environment_id TEXT;

CREATE TABLE IF NOT EXISTS compose_services (
    id TEXT PRIMARY KEY,
    stack_id TEXT NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    image TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    state TEXT NOT NULL DEFAULT '',
    ports TEXT NOT NULL DEFAULT '',
    health TEXT NOT NULL DEFAULT '',
    node_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compose_services_stack_idx ON compose_services (stack_id);
CREATE INDEX IF NOT EXISTS compose_services_node_idx ON compose_services (node_id);

CREATE TABLE IF NOT EXISTS compose_logs (
    id BIGSERIAL PRIMARY KEY,
    stack_id TEXT NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    service_name TEXT NOT NULL DEFAULT '',
    stream TEXT NOT NULL DEFAULT 'stdout',
    message TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compose_logs_stack_idx ON compose_logs (stack_id, timestamp DESC);

-- Environment reference index
CREATE INDEX IF NOT EXISTS compose_stacks_env_idx ON compose_stacks (environment_id) WHERE environment_id IS NOT NULL;
