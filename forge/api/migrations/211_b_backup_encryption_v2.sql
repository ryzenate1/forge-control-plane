-- BK-03 / BK-04: backup crypto V2 — per-backup salt + AAD binding + chunked streaming
-- Adds per-backup salt and version columns for V2 (BACKUP_ENCRYPTION_V2).
-- Old backups keep legacy nonce||ct fallback (encryption_version=1, no salt/AAD).
-- New backups use salt 16B + AAD server_id:backup_name + 1MiB chunked seals.
ALTER TABLE backups
    ADD COLUMN IF NOT EXISTS encryption_salt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS encryption_version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS encryption_aad TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN backups.encryption_salt IS 'Hex-encoded 16B per-backup random salt for HKDF (V2)';
COMMENT ON COLUMN backups.encryption_version IS '1=legacy nonce||ct, 2=salt+AAD+chunked';
COMMENT ON COLUMN backups.encryption_aad IS 'AAD binding value server_id:backup_name for V2';

ALTER TABLE backup_artifacts
    ADD COLUMN IF NOT EXISTS encryption_salt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS encryption_version INT NOT NULL DEFAULT 1;

COMMENT ON COLUMN backup_artifacts.encryption_salt IS 'Per-artifact salt for V2';
-- Existing encrypted flag remains; _encrypted suffix not needed for blob salt (stored as hex, not keyring secret)
-- For keyring-encrypted secrets, ensure _encrypted column exists for storage provider configs (already exists as config_encrypted)

-- Ensure index for GC reaper
CREATE INDEX IF NOT EXISTS idx_backups_partial_gc ON backups(server_id, created_at) WHERE name LIKE '%.partial';
