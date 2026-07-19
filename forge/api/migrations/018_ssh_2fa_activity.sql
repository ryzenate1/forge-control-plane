-- SSH Keys for user accounts.
CREATE TABLE IF NOT EXISTS user_ssh_keys (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    public_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_ssh_keys_user_id_idx ON user_ssh_keys (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS user_ssh_keys_fingerprint_idx ON user_ssh_keys (user_id, fingerprint);

-- Two-factor authentication fields on users table.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS use_totp BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS totp_secret TEXT,
    ADD COLUMN IF NOT EXISTS totp_authenticated_at TIMESTAMPTZ;

-- Recovery tokens for 2FA backup codes.
CREATE TABLE IF NOT EXISTS recovery_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS recovery_tokens_user_id_idx ON recovery_tokens (user_id);

-- Activity log table for detailed user/server activity tracking.
CREATE TABLE IF NOT EXISTS activity_logs (
    id UUID PRIMARY KEY,
    batch UUID,
    event TEXT NOT NULL,
    ip TEXT,
    description TEXT,
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_type TEXT NOT NULL DEFAULT 'user',
    subject_type TEXT,
    subject_id UUID,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS activity_logs_actor_idx ON activity_logs (actor_id);
CREATE INDEX IF NOT EXISTS activity_logs_event_idx ON activity_logs (event);
CREATE INDEX IF NOT EXISTS activity_logs_subject_idx ON activity_logs (subject_type, subject_id);
CREATE INDEX IF NOT EXISTS activity_logs_timestamp_idx ON activity_logs (timestamp DESC);
