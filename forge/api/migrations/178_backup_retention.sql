-- Phase 3: backup retention policy. Keyed on engine kind (the same strings
-- stored in managed_databases.engine), a row enables the hourly retention
-- worker to delete old managed database backups. retention_days removes
-- backups older than the cutoff; retention_max always keeps the newest N
-- completed backups per database regardless of age.
CREATE TABLE IF NOT EXISTS backup_retention (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind VARCHAR(50) UNIQUE NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    retention_days INT NOT NULL DEFAULT 30,
    retention_max INT NOT NULL DEFAULT 8,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_managed_database_backups_created ON managed_database_backups(created_at);
CREATE INDEX IF NOT EXISTS idx_managed_database_backups_engine ON managed_database_backups(engine);

INSERT INTO backup_retention (kind, enabled, retention_days, retention_max) VALUES
    ('postgresql', TRUE, 30, 8),
    ('mysql', TRUE, 30, 8),
    ('mariadb', TRUE, 30, 8),
    ('redis', TRUE, 30, 8),
    ('mongodb', TRUE, 30, 8)
ON CONFLICT (kind) DO NOTHING;