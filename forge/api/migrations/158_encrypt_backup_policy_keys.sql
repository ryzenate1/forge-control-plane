ALTER TABLE backup_policies
    ADD COLUMN IF NOT EXISTS encryption_key_encrypted TEXT NOT NULL DEFAULT '';
