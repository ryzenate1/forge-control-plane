-- Phase 3: catalog connection-string attach links. Records which environment
-- a catalog instance's connection variables were injected into, plus the
-- variable prefix used (DATABASE_URL, REDIS_URL, AMQP_URL, ...).
CREATE TABLE IF NOT EXISTS catalog_attach_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    catalog_instance_id UUID NOT NULL REFERENCES catalog_instances(id) ON DELETE CASCADE,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    var_prefix VARCHAR(50) NOT NULL DEFAULT 'CATALOG',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (catalog_instance_id, environment_id)
);

CREATE INDEX IF NOT EXISTS idx_catalog_attach_links_env ON catalog_attach_links(environment_id);