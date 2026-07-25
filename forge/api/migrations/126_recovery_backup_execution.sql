-- Backup-only recovery is distinct from live migration. These states record
-- verified archive restoration without asserting server ownership moved.
-- Note: 'executing', 'completed', 'skipped' are added by migration 080
ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'restored';

ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'restored';

ALTER TABLE recovery_items
    ADD COLUMN IF NOT EXISTS source_backup_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_backup_checksum text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_backup_size bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_backup_uuid UUID REFERENCES backups(uuid) ON DELETE SET NULL;
