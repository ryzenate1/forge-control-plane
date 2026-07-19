CREATE TABLE IF NOT EXISTS activity_events (
    id TEXT PRIMARY KEY,
    event TEXT NOT NULL,
    description TEXT,
    actor_id TEXT,
    actor_email TEXT,
    actor_type TEXT NOT NULL DEFAULT 'user',
    ip TEXT,
    user_agent TEXT,
    subject_type TEXT,
    subject_id TEXT,
    subject_name TEXT,
    properties TEXT NOT NULL DEFAULT '{}',
    level TEXT NOT NULL DEFAULT 'info',
    source TEXT NOT NULL DEFAULT 'api',
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_activity_events_timestamp ON activity_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_activity_events_actor ON activity_events(actor_id);
CREATE INDEX IF NOT EXISTS idx_activity_events_subject ON activity_events(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_activity_events_event ON activity_events(event);
CREATE INDEX IF NOT EXISTS idx_activity_events_level ON activity_events(level);
CREATE INDEX IF NOT EXISTS idx_activity_events_source ON activity_events(source);
CREATE INDEX IF NOT EXISTS idx_activity_events_expires ON activity_events(expires_at);
