-- 083_a_traffic_rules.sql (MySQL/MariaDB dialect)
--
-- uuid/jsonb/timestamptz/TEXT[] are rewritten by mysqlCompatibleMigration; this
-- override exists for the parts a blind token rewrite cannot express:
--   * ip_whitelist / ip_blacklist are TEXT[] in canonical. The translator degrades
--     TEXT[] to plain TEXT, which stores neither a valid array nor valid JSON, so
--     the columns are declared JSON here instead. mysql/083_a must stay in step
--     with internal/store/store_traffic.go, which reads ip_whitelist/ip_blacklist
--     into []string and writes them back as a driver parameter.
--   * the canonical FK and the DEFAULT-less server_id are preserved below (the
--     previous override silently dropped the FK and invented DEFAULT '').
CREATE TABLE IF NOT EXISTS traffic_rules (
    id          CHAR(36) PRIMARY KEY,
    name        TEXT NOT NULL,
    server_id   CHAR(36) NOT NULL,
    domain      TEXT NOT NULL,
    path        TEXT NOT NULL DEFAULT '/',
    target_host TEXT NOT NULL DEFAULT '',
    target_port INTEGER NOT NULL,
    protocol    TEXT NOT NULL DEFAULT 'http',
    strategy    TEXT NOT NULL DEFAULT 'round_robin',
    weight      INTEGER NOT NULL DEFAULT 1,
    headers     JSON NOT NULL DEFAULT ('{}'),
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    web_socket  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_traffic_rules_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS traffic_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    rate_limit INTEGER NOT NULL DEFAULT 0,
    rate_limit_burst INTEGER NOT NULL DEFAULT 0,
    ip_whitelist JSON NOT NULL DEFAULT ('[]'),
    ip_blacklist JSON NOT NULL DEFAULT ('[]'),
    tls_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    tls_cert_file TEXT NOT NULL DEFAULT '',
    tls_key_file TEXT NOT NULL DEFAULT '',
    circuit_breaker BOOLEAN NOT NULL DEFAULT FALSE,
    circuit_breaker_threshold INTEGER NOT NULL DEFAULT 0,
    circuit_breaker_timeout INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);
