-- 124_activity_events.sql (MySQL/MariaDB dialect)
--
-- The canonical file needs a dialect override for two reasons:
--   1. `CREATE INDEX ... WHERE expires_at IS NOT NULL` is a partial index, which
--      neither MySQL nor MariaDB supports; the override creates the same index
--      unconditionally (a plain index on a nullable column is a superset, not a
--      weakening, of the partial one).
--   2. uuid/inet/jsonb/timestamptz/now() are handled by mysqlCompatibleMigration,
--      but are written out here so this file is reviewable as MySQL on its own.
--
-- This DDL previously lived in mysql/054_activity_events.sql, which is the
-- no-op GUARD side of the 054 -> 124 rename alias (migrationAliases in
-- internal/store/migration.go). Carrying schema in the guard copy inverted the
-- guard contract ("a guard row proves nothing"): on a fresh MySQL host the guard
-- executed the DDL, and canonical 124 -- untranslatable because of the partial
-- index -- then aborted the run, so schema_migrations never recorded 124.
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
    `timestamp` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL
);

CREATE INDEX IF NOT EXISTS idx_activity_events_timestamp ON activity_events(`timestamp` DESC);
CREATE INDEX IF NOT EXISTS idx_activity_events_actor ON activity_events(actor_id);
CREATE INDEX IF NOT EXISTS idx_activity_events_subject ON activity_events(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_activity_events_event ON activity_events(event);
CREATE INDEX IF NOT EXISTS idx_activity_events_level ON activity_events(level);
CREATE INDEX IF NOT EXISTS idx_activity_events_source ON activity_events(source);
CREATE INDEX IF NOT EXISTS idx_activity_events_expires ON activity_events(expires_at);
