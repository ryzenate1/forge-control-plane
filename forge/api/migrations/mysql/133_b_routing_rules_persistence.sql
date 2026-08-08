CREATE TABLE IF NOT EXISTS traffic_rules (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    server_id   TEXT NOT NULL DEFAULT '',
    domain      TEXT NOT NULL DEFAULT '',
    path        TEXT NOT NULL DEFAULT '/',
    target_host TEXT NOT NULL DEFAULT '',
    target_port INTEGER NOT NULL DEFAULT 80,
    protocol    TEXT NOT NULL DEFAULT 'http',
    strategy    TEXT NOT NULL DEFAULT 'round_robin',
    weight      INTEGER NOT NULL DEFAULT 1,
    headers     JSON NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    web_socket  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);