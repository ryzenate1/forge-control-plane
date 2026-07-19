-- Canonical webhook schema. The original migration was misplaced under
-- internal/store/migrations and is not discovered by the migration runner.
CREATE TABLE IF NOT EXISTS webhooks (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    url                TEXT NOT NULL,
    webhook_type       TEXT NOT NULL DEFAULT 'regular' CHECK (webhook_type IN ('regular', 'discord')),
    events             TEXT[] NOT NULL DEFAULT '{}',
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    secret             TEXT NOT NULL DEFAULT '',
    discord_username   TEXT NOT NULL DEFAULT '',
    discord_avatar_url TEXT NOT NULL DEFAULT '',
    discord_content    TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS webhooks_enabled_idx ON webhooks (enabled);
