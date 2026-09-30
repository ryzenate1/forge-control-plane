-- Runtime-configurable cloud providers added via the admin API.
-- Only one active provider per kind is supported, matching the in-memory
-- cloud Manager which is keyed by provider kind. The secret access key is
-- stored encrypted at rest via the panel keyring.
CREATE TABLE IF NOT EXISTS cloud_providers (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              TEXT NOT NULL UNIQUE,
    name              TEXT NOT NULL DEFAULT '',
    region            TEXT NOT NULL,
    access_key_id     TEXT NOT NULL DEFAULT '',
    secret_encrypted  TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
