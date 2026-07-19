-- Add backup locking feature to prevent accidental deletion of critical backups
-- This migration adds is_locked column to backups table

ALTER TABLE backups
ADD COLUMN IF NOT EXISTS is_locked BOOLEAN NOT NULL DEFAULT FALSE;

-- Add comment
COMMENT ON COLUMN backups.is_locked IS 'Prevents accidental deletion of critical backups when set to true';

-- Add index for faster locked backup queries
CREATE INDEX IF NOT EXISTS backups_server_id_locked_idx ON backups (server_id, is_locked);
