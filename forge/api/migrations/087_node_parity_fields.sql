-- Node parity columns (Pelican parity). All statements use
-- ADD COLUMN IF NOT EXISTS so re-runs and the 129 duplicate converge.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS daemon_sftp_alias VARCHAR(255);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS daemon_connect INT DEFAULT 8080;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS cpu_overallocate INT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
