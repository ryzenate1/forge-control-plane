CREATE TABLE IF NOT EXISTS service_discovery_endpoints (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL,
    service_id      TEXT NOT NULL DEFAULT '',
    node_id         TEXT NOT NULL,
    node_name       TEXT NOT NULL DEFAULT '',
    region_id       TEXT,
    address         TEXT NOT NULL,
    port            INTEGER NOT NULL,
    protocol        TEXT NOT NULL DEFAULT 'tcp',
    status          TEXT NOT NULL DEFAULT 'unknown',
    replica_index   INTEGER NOT NULL DEFAULT 0,
    tenant_id       TEXT,
    last_heartbeat  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_svc_disc_svc_name ON service_discovery_endpoints (service_name);
CREATE INDEX IF NOT EXISTS idx_svc_disc_node_id  ON service_discovery_endpoints (node_id);
CREATE INDEX IF NOT EXISTS idx_svc_disc_tenant_id ON service_discovery_endpoints (tenant_id);
