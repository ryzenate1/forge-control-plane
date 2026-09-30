-- store_restores.go reads/writes a JSONB `data` column on backup_restores
-- (restore payload/options blob), but no migration ever created it, so
-- GET /admin/backups/restores and the backup status rollup failed with
-- `column "data" does not exist`. Added idempotently to match 223's style.
ALTER TABLE backup_restores
    ADD COLUMN IF NOT EXISTS data JSONB;
