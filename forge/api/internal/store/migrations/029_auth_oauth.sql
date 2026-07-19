-- OAuth2 client registration and token revocation.

CREATE TABLE IF NOT EXISTS oauth_clients (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id TEXT NOT NULL UNIQUE,
    client_secret_hash TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'account',
    server_id UUID,
    allowed_scopes TEXT[] NOT NULL DEFAULT '{}',
    description TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_clients_owner_id_idx ON oauth_clients (owner_id);
CREATE INDEX IF NOT EXISTS oauth_clients_server_id_idx ON oauth_clients (server_id) WHERE server_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS oauth_revoked_tokens (
    jti TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS oauth_revoked_tokens_expires_at_idx ON oauth_revoked_tokens (expires_at);
