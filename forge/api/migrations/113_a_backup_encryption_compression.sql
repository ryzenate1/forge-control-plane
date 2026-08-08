-- Add backup encryption and compression metadata columns
ALTER TABLE backups
    ADD COLUMN IF NOT EXISTS compressed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS nonce TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN backups.compressed IS 'Whether the backup data is gzip-compressed';
COMMENT ON COLUMN backups.encrypted IS 'Whether the backup data is AES-256-GCM encrypted';
COMMENT ON COLUMN backups.nonce IS 'Hex-encoded 12-byte nonce used for encryption';
