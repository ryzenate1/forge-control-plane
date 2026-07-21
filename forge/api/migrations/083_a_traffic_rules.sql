CREATE TABLE IF NOT EXISTS traffic_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    server_id TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL,
    path TEXT NOT NULL DEFAULT '/',
    target_host TEXT NOT NULL DEFAULT '',
    target_port INTEGER NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'http',
    strategy TEXT NOT NULL DEFAULT 'round_robin',
    weight INTEGER NOT NULL DEFAULT 1,
    headers JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    web_socket BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS traffic_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    rate_limit INTEGER NOT NULL DEFAULT 0,
    rate_limit_burst INTEGER NOT NULL DEFAULT 0,
    ip_whitelist TEXT[] NOT NULL DEFAULT '{}',
    ip_blacklist TEXT[] NOT NULL DEFAULT '{}',
    tls_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    tls_cert_file TEXT NOT NULL DEFAULT '',
    tls_key_file TEXT NOT NULL DEFAULT '',
    circuit_breaker BOOLEAN NOT NULL DEFAULT FALSE,
    circuit_breaker_threshold INTEGER NOT NULL DEFAULT 0,
    circuit_breaker_timeout INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);
