-- Social authentication providers and linked identities.

CREATE TABLE IF NOT EXISTS social_providers (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    client_id TEXT NOT NULL DEFAULT '',
    client_secret TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS social_identities (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES social_providers(id) ON DELETE CASCADE,
    provider_user_id TEXT NOT NULL,
    provider_email TEXT,
    provider_username TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, provider_user_id)
);

CREATE INDEX IF NOT EXISTS social_identities_user_id_idx ON social_identities (user_id);
CREATE INDEX IF NOT EXISTS social_identities_provider_idx ON social_identities (provider_id, provider_user_id);
