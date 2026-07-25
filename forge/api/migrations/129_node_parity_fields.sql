-- Duplicate of 087_node_parity_fields.sql; kept for installs that recorded
-- only this version, so it must be safe to re-run on an up-to-date schema.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS daemon_sftp_alias VARCHAR(255);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS daemon_connect INT DEFAULT 8080;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS cpu_overallocate INT DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
