-- Add scoped permissions to API keys.
-- Stored as a JSON array of permission strings, e.g. ["servers.read","nodes.read"].
-- An empty/null array means NO permissions (fail-closed).
-- The special scope "*" grants full access.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS scopes JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Add allowed_ips for IP restriction
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS allowed_ips TEXT[] NOT NULL DEFAULT '{}';
