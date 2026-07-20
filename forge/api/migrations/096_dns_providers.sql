CREATE TABLE IF NOT EXISTS dns_providers (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    provider_type   TEXT NOT NULL,
    credentials     TEXT NOT NULL DEFAULT '',
    credentials_encrypted TEXT NOT NULL DEFAULT '',
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    verified        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS dns_providers_default_idx ON dns_providers (is_default) WHERE is_default = TRUE;
