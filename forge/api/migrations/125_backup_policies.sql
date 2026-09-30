-- backup_policies is defined in migration 119_z_backup_policies.sql
-- This migration adds constraints and indexes.

DO $$ BEGIN ALTER TABLE backup_policies ADD CONSTRAINT backup_policies_storage_check CHECK (storage IN ('s3', 'local', 'sftp', 'gcs', 'azure')); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN ALTER TABLE backup_policies ADD CONSTRAINT backup_policies_interval_check CHECK (interval ~ '^(\d+\s+(minute|hour|day|week|month)s?|@(daily|weekly|monthly|yearly)|(\d+|\*)(/\d+)?(\s+\d+|\s+\*){4,5})$'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_backup_policies_server ON backup_policies(server_id);
