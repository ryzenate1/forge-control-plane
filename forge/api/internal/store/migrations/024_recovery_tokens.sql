CREATE TABLE IF NOT EXISTS recovery_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb DEFAULT '{}',
    ip inet,
    user_agent text
);

CREATE INDEX IF NOT EXISTS idx_recovery_tokens_user ON recovery_tokens(user_id, type);
CREATE INDEX IF NOT EXISTS idx_recovery_tokens_hash ON recovery_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_recovery_tokens_expires ON recovery_tokens(expires_at) WHERE used_at IS NULL;
