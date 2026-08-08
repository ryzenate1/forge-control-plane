-- Phase 3: catalog instance rows. Each row tracks one provisioned service
-- created from the one-click catalog and records where its runtime lives
-- (db_container row or compose stack) plus the connection string derived
-- from it.
CREATE TABLE IF NOT EXISTS catalog_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_key VARCHAR(100) NOT NULL REFERENCES catalog_entries(key) ON DELETE CASCADE,
    kind VARCHAR(50) NOT NULL,
    version VARCHAR(50) NOT NULL DEFAULT '',
    environment_id UUID REFERENCES environments(id) ON DELETE SET NULL,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    ref_type VARCHAR(30) NOT NULL DEFAULT '',
    instance_ref UUID,
    host VARCHAR(255) NOT NULL DEFAULT '',
    port INT NOT NULL DEFAULT 0,
    conn_string TEXT NOT NULL DEFAULT '',
    status VARCHAR(30) NOT NULL DEFAULT 'provisioning',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_catalog_instances_env ON catalog_instances(environment_id);
CREATE INDEX IF NOT EXISTS idx_catalog_instances_entry ON catalog_instances(entry_key);
CREATE INDEX IF NOT EXISTS idx_catalog_instances_status ON catalog_instances(status);