CREATE TABLE IF NOT EXISTS activity_events (
    id CHAR(36) PRIMARY KEY,
    event TEXT NOT NULL,
    description TEXT,
    actor_id CHAR(36),
    actor_email TEXT,
    actor_type TEXT NOT NULL DEFAULT 'user',
    ip VARCHAR(45),
    user_agent TEXT,
    subject_type TEXT,
    subject_id CHAR(36),
    subject_name TEXT,
    properties JSON NOT NULL DEFAULT ('{}'),
    level TEXT NOT NULL DEFAULT 'info',
    source TEXT NOT NULL DEFAULT 'api',
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL
);

CREATE INDEX idx_activity_events_timestamp ON activity_events(timestamp DESC);
CREATE INDEX idx_activity_events_actor ON activity_events(actor_id);
CREATE INDEX idx_activity_events_subject ON activity_events(subject_type, subject_id);
CREATE INDEX idx_activity_events_event ON activity_events(event);
CREATE INDEX idx_activity_events_level ON activity_events(level);
CREATE INDEX idx_activity_events_source ON activity_events(source);
CREATE INDEX idx_activity_events_expires ON activity_events(expires_at);
