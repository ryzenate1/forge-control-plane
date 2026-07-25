ALTER TABLE backup_storage_providers
    ADD COLUMN IF NOT EXISTS config_encrypted TEXT NOT NULL DEFAULT '';

UPDATE backup_storage_providers
SET config = '{}'::jsonb
WHERE config IS NULL;
