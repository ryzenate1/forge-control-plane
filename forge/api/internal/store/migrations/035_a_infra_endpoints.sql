CREATE TABLE IF NOT EXISTS infra_endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    endpoint_type TEXT NOT NULL DEFAULT 'docker',
    connection_mode TEXT NOT NULL DEFAULT 'direct',
    status TEXT NOT NULL DEFAULT 'unknown',
    edge_id TEXT,
    tls_ca TEXT,
    tls_cert TEXT,
    tls_key TEXT,
    tags TEXT[] DEFAULT '{}',
    labels JSONB DEFAULT '[]',
    url TEXT DEFAULT '',
    project_id TEXT,
    group_id TEXT,
    reachable BOOLEAN DEFAULT false,
    version TEXT DEFAULT '',
    total_container_count INTEGER DEFAULT 0,
    total_image_count INTEGER DEFAULT 0,
    total_volume_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS infra_endpoint_nodes (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(endpoint_id, node_id)
);

CREATE TABLE IF NOT EXISTS infra_endpoint_access_policies (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(endpoint_id, principal_type, principal_id)
);

CREATE TABLE IF NOT EXISTS infra_endpoint_health_history (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    reachable BOOLEAN NOT NULL DEFAULT false,
    health_score REAL DEFAULT 0,
    version TEXT DEFAULT '',
    total_containers INTEGER DEFAULT 0,
    total_images INTEGER DEFAULT 0,
    total_volumes INTEGER DEFAULT 0,
    error_message TEXT DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_endpoint_nodes_endpoint ON infra_endpoint_nodes(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_nodes_node ON infra_endpoint_nodes(node_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_policies_endpoint ON infra_endpoint_access_policies(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_health_endpoint ON infra_endpoint_health_history(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_health_observed ON infra_endpoint_health_history(endpoint_id, observed_at DESC);
