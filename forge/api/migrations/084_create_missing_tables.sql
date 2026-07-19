-- Create missing tables referenced by store code.
-- All use CREATE TABLE IF NOT EXISTS for idempotency.

-- 1) audit_logs - event auditing per user/resource
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    details JSONB DEFAULT '{}'::jsonb,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs (resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs (created_at DESC);

-- 2) notifications - in-app notifications per user
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    read BOOLEAN NOT NULL DEFAULT false,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_read ON notifications (user_id, read);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications (created_at DESC);

-- 3) notification_preferences - per-user channel/event-type opt-in
CREATE TABLE IF NOT EXISTS notification_preferences (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    event_type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (user_id, channel, event_type)
);

CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_id ON notification_preferences (user_id);

-- 4) sftp_node_configs - per-node SFTP overrides
CREATE TABLE IF NOT EXISTS sftp_node_configs (
    node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    listen_port INTEGER NOT NULL DEFAULT 2022,
    listen_ip TEXT,
    max_connections INTEGER NOT NULL DEFAULT 10,
    max_auth_attempts INTEGER NOT NULL DEFAULT 3,
    idle_timeout INTEGER NOT NULL DEFAULT 300,
    rate_limit INTEGER NOT NULL DEFAULT 0,
    read_only BOOLEAN NOT NULL DEFAULT false,
    allowed_ips TEXT[] DEFAULT '{}',
    banner TEXT,
    log_level TEXT NOT NULL DEFAULT 'info',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 5) sftp_global_config - singleton global SFTP defaults
CREATE TABLE IF NOT EXISTS sftp_global_config (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT true,
    default_port INTEGER NOT NULL DEFAULT 2022,
    default_max_connections INTEGER NOT NULL DEFAULT 10,
    default_idle_timeout INTEGER NOT NULL DEFAULT 300,
    default_rate_limit INTEGER NOT NULL DEFAULT 0,
    log_level TEXT NOT NULL DEFAULT 'info',
    allowed_ciphers TEXT[] DEFAULT '{}',
    allowed_macs TEXT[] DEFAULT '{}',
    allowed_kex_algos TEXT[] DEFAULT '{}',
    host_key_algorithms TEXT[] DEFAULT '{}'
);

-- Seed default global config row if missing.
INSERT INTO sftp_global_config (id, enabled, default_port, default_max_connections,
    default_idle_timeout, default_rate_limit, log_level)
VALUES (1, true, 2022, 10, 300, 0, 'info')
ON CONFLICT (id) DO NOTHING;

-- 6) Cleanup: drop zombie columns from nodes table.
-- Migration 007 added daemon_listen_port / daemon_sftp_port.
-- Migration 012 added daemon_listen / daemon_sftp as replacements.
ALTER TABLE nodes DROP COLUMN IF EXISTS daemon_listen_port;
ALTER TABLE nodes DROP COLUMN IF EXISTS daemon_sftp_port;
