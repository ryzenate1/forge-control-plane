ALTER TABLE backup_configurations
    ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE backup_jobs
    ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE backup_artifacts
    ADD COLUMN IF NOT EXISTS data JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_backup_configurations_name_lower
    ON backup_configurations (lower(name));

CREATE INDEX IF NOT EXISTS idx_backup_jobs_name_lower
    ON backup_jobs (lower(name));

CREATE INDEX IF NOT EXISTS idx_backup_artifacts_name_lower
    ON backup_artifacts (lower(name));
