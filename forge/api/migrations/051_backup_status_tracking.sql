-- Add backup status tracking fields
ALTER TABLE backups
ADD COLUMN IF NOT EXISTS status_message TEXT,
ADD COLUMN IF NOT EXISTS status_callback TEXT,
ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_retry_at TIMESTAMP WITH TIME ZONE;

-- Create index for callback-based lookups
CREATE INDEX IF NOT EXISTS idx_backups_status_callback ON backups(status_callback) WHERE status_callback IS NOT NULL;
