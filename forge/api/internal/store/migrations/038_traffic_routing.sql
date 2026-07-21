CREATE TABLE IF NOT EXISTS traffic_rules (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    server_id  TEXT REFERENCES servers(id) ON DELETE CASCADE,
    domain     TEXT NOT NULL DEFAULT '',
    path       TEXT NOT NULL DEFAULT '/',
    target_port INTEGER NOT NULL DEFAULT 80,
    protocol   TEXT NOT NULL DEFAULT 'http',
    strategy   TEXT NOT NULL DEFAULT 'round_robin',
    weight     INTEGER NOT NULL DEFAULT 0,
    headers    JSONB DEFAULT '{}',
    enabled    BOOLEAN NOT NULL DEFAULT false,
    web_socket BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS traffic_rules_server_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_domain_idx ON traffic_rules (domain);

CREATE TABLE IF NOT EXISTS traffic_policies (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL DEFAULT '',
    rate_limit               INTEGER NOT NULL DEFAULT 0,
    rate_limit_burst         INTEGER NOT NULL DEFAULT 0,
    ip_whitelist             TEXT[] DEFAULT '{}',
    ip_blacklist             TEXT[] DEFAULT '{}',
    tls_enabled              BOOLEAN NOT NULL DEFAULT false,
    tls_cert_file            TEXT NOT NULL DEFAULT '',
    tls_key_file             TEXT NOT NULL DEFAULT '',
    circuit_breaker          BOOLEAN NOT NULL DEFAULT false,
    circuit_breaker_threshold INTEGER NOT NULL DEFAULT 5,
    circuit_breaker_timeout  INTEGER NOT NULL DEFAULT 30,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS traffic_policies_name_idx ON traffic_policies (name);
