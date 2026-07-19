-- Add S3 backup configuration to panel settings
-- This migration adds columns for S3 backup configuration with encryption support

ALTER TABLE panel_settings
ADD COLUMN IF NOT EXISTS s3_backup_enabled BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS s3_endpoint TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_region TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_bucket TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_access_key_id TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_secret_access_key TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_secret_access_key_encrypted TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_prefix TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS s3_use_path_style BOOLEAN NOT NULL DEFAULT TRUE;

-- Add comments
COMMENT ON COLUMN panel_settings.s3_backup_enabled IS 'Enable S3 as backup storage provider';
COMMENT ON COLUMN panel_settings.s3_endpoint IS 'S3 endpoint URL (optional, for S3-compatible storage)';
COMMENT ON COLUMN panel_settings.s3_region IS 'S3 region';
COMMENT ON COLUMN panel_settings.s3_bucket IS 'S3 bucket name';
COMMENT ON COLUMN panel_settings.s3_access_key_id IS 'S3 access key ID';
COMMENT ON COLUMN panel_settings.s3_secret_access_key IS 'S3 secret access key (plaintext, for migration)';
COMMENT ON COLUMN panel_settings.s3_secret_access_key_encrypted IS 'S3 secret access key (encrypted)';
COMMENT ON COLUMN panel_settings.s3_prefix IS 'S3 key prefix for backups';
COMMENT ON COLUMN panel_settings.s3_use_path_style IS 'Use path-style S3 URLs (true for MinIO/compatible storage)';
