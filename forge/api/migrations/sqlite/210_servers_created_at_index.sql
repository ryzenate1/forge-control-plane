-- Phase 10: hot-path index for ListServers* pagination (SQLite dialect).
CREATE INDEX IF NOT EXISTS idx_servers_created_at ON servers (created_at DESC);
