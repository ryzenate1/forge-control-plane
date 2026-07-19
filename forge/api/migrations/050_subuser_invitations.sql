-- Subuser invitations table
CREATE TABLE IF NOT EXISTS subuser_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    permissions TEXT[] NOT NULL,
    token VARCHAR(64) NOT NULL UNIQUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    accepted_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE
);

-- Index for faster lookups
CREATE INDEX IF NOT EXISTS idx_subuser_invitations_token ON subuser_invitations(token);
CREATE INDEX IF NOT EXISTS idx_subuser_invitations_server ON subuser_invitations(server_id);
CREATE INDEX IF NOT EXISTS idx_subuser_invitations_email ON subuser_invitations(email);

-- Index for cleanup of expired invitations
CREATE INDEX IF NOT EXISTS idx_subuser_invitations_expires ON subuser_invitations(expires_at);
