-- OAuth2 server (PufferPanel parity). Clients register with the panel and
-- exchange client_credentials for short-lived access tokens scoped to a
-- specific server (server-scoped client) or to the user's account
-- (account-scoped client). Tokens are JWTs signed with the panel secret.

CREATE TABLE IF NOT EXISTS oauth_clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id TEXT NOT NULL UNIQUE,
    client_secret_hash TEXT NOT NULL,
    -- "server" or "account" — server-scoped clients only get the server's
    -- allowed scopes; account-scoped clients get the owner's full perm set.
    scope TEXT NOT NULL CHECK (scope IN ('server', 'account')),
    -- For server-scoped clients, the server they grant access to. NULL for
    -- account-scoped clients.
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    -- JSON array of OAuth scope strings the client is allowed to request.
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Optional description for the admin UI.
    description TEXT,
    -- When set, the client cannot mint new tokens after this time.
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (scope = 'server' AND server_id IS NOT NULL) OR
        (scope = 'account' AND server_id IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS oauth_clients_owner_idx ON oauth_clients (owner_id);
CREATE INDEX IF NOT EXISTS oauth_clients_server_idx ON oauth_clients (server_id) WHERE server_id IS NOT NULL;

-- Token revocation list (optional, in addition to JWT expiry).
CREATE TABLE IF NOT EXISTS oauth_revoked_tokens (
    jti TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_revoked_tokens_expires_idx ON oauth_revoked_tokens (expires_at);