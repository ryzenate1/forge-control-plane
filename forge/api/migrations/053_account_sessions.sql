-- Account Sessions Table
-- Stores user sessions for session management (list/revoke functionality)
-- References standard session management pattern

CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash TEXT NOT NULL,
    ip_address TEXT,
    user_agent TEXT,
    last_activity TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoke_reason TEXT
);

-- Index for efficient session lookups by user
CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id) WHERE NOT is_revoked;

-- Index for token-based session lookup
CREATE INDEX idx_user_sessions_token_hash ON user_sessions(session_token_hash) WHERE NOT is_revoked;

-- Index for expired session cleanup
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at) WHERE NOT is_revoked;

-- Index for last activity sorting
CREATE INDEX idx_user_sessions_last_activity ON user_sessions(user_id, last_activity DESC) WHERE NOT is_revoked;

-- Function to clean up expired sessions
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS void AS $$
BEGIN
    DELETE FROM user_sessions
    WHERE expires_at < NOW() OR (is_revoked = TRUE AND revoked_at < NOW() - INTERVAL '30 days');
END;
$$ LANGUAGE plpgsql;

-- Schedule cleanup to run daily (requires pg_cron extension, otherwise manual cleanup needed)
-- Uncomment if pg_cron is available:
-- SELECT cron.schedule('cleanup-expired-sessions', '0 2 * * *', 'SELECT cleanup_expired_sessions()');

-- Trigger to update last_activity on session access
CREATE OR REPLACE FUNCTION update_session_last_activity()
RETURNS trigger AS $$
BEGIN
    NEW.last_activity = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Comment for documentation
COMMENT ON TABLE user_sessions IS 'Stores user authentication sessions for session management';
COMMENT ON COLUMN user_sessions.session_token_hash IS 'SHA-256 hash of the session token for secure lookup';
COMMENT ON COLUMN user_sessions.is_revoked IS 'Whether the session has been revoked by the user';
COMMENT ON COLUMN user_sessions.revoke_reason IS 'Optional reason for session revocation (e.g., "Security", "Logout")';
