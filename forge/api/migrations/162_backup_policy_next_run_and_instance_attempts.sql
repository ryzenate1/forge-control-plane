ALTER TABLE backup_policies ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ;
ALTER TABLE instances ADD COLUMN IF NOT EXISTS replacement_attempts INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_backup_policies_next_run ON backup_policies(next_run_at);
