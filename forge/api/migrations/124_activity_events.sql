-- Activity Events Table
-- Enhanced activity logging with structured event data, levels, sources, and expiry for automatic cleanup.
-- Supersedes the simpler activity_logs and audit_events tables with richer metadata.

CREATE TABLE IF NOT EXISTS activity_events (
    id uuid PRIMARY KEY,
    event text NOT NULL,
    description text,
    actor_id uuid,
    actor_email text,
    actor_type text NOT NULL DEFAULT 'user',
    ip inet,
    user_agent text,
    subject_type text,
    subject_id uuid,
    subject_name text,
    properties jsonb NOT NULL DEFAULT '{}',
    level text NOT NULL DEFAULT 'info',
    source text NOT NULL DEFAULT 'api',
    timestamp timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz
);

CREATE INDEX idx_activity_events_timestamp ON activity_events(timestamp DESC);
CREATE INDEX idx_activity_events_actor ON activity_events(actor_id);
CREATE INDEX idx_activity_events_subject ON activity_events(subject_type, subject_id);
CREATE INDEX idx_activity_events_event ON activity_events(event);
CREATE INDEX idx_activity_events_level ON activity_events(level);
CREATE INDEX idx_activity_events_source ON activity_events(source);
CREATE INDEX idx_activity_events_expires ON activity_events(expires_at) WHERE expires_at IS NOT NULL;
