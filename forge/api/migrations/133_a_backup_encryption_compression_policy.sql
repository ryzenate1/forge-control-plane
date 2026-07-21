ALTER TABLE backup_policies
    ADD COLUMN IF NOT EXISTS compress BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS encryption_algorithm TEXT NOT NULL DEFAULT 'aes-256-gcm',
    ADD COLUMN IF NOT EXISTS encryption_key TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN backup_policies.compress IS 'Whether to gzip-compress backup data';
COMMENT ON COLUMN backup_policies.encrypted IS 'Whether to AES-256-GCM encrypt backup data';
COMMENT ON COLUMN backup_policies.encryption_algorithm IS 'Encryption algorithm identifier';
COMMENT ON COLUMN backup_policies.encryption_key IS 'Reference or value of the encryption key';
