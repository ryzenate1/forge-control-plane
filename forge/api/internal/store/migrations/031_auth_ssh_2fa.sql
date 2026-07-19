-- SSH keys for user accounts and two-factor authentication tokens.

CREATE TABLE IF NOT EXISTS user_ssh_keys (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    fingerprint TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_ssh_keys_user_id_idx ON user_ssh_keys (user_id);
CREATE INDEX IF NOT EXISTS user_ssh_keys_fingerprint_idx ON user_ssh_keys (fingerprint);

CREATE TABLE IF NOT EXISTS two_factor_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret TEXT NOT NULL,
    confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS two_factor_tokens_user_id_idx ON two_factor_tokens (user_id);
