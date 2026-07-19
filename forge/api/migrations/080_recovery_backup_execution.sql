-- Backup-only recovery is distinct from live migration. These states record
-- verified archive restoration without asserting server ownership moved.
ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'executing';
ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'completed';
ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'restored';

ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'executing';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'completed';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'restored';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'skipped';

ALTER TABLE recovery_items
    ADD COLUMN IF NOT EXISTS source_backup_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_backup_checksum text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_backup_size bigint NOT NULL DEFAULT 0;
