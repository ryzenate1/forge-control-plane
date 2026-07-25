-- Phase 10: hot-path index for ListServers* pagination (ORDER BY s.created_at DESC).
-- ListServers, ListServersPaginated and ListServersForOrg all sort by created_at DESC
-- with limit/offset clamped to 100. Without a dedicated index the sort spills to
-- sequential scan as the servers table grows.
CREATE INDEX IF NOT EXISTS idx_servers_created_at ON servers (created_at DESC);
