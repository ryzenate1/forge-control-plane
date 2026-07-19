ALTER TABLE backups ADD COLUMN deleted_at TIMESTAMP;
CREATE INDEX IF NOT EXISTS idx_backups_deleted_at ON backups(deleted_at);
