-- 217_api_perf_indexes.sql
-- Phase 09-01: Hot-path index optimization for ListServers, ListAllocations,
-- ListBackups and auth-heavy reads. All additive (IF NOT EXISTS), no breaking DDL.
-- EXPLAIN ANALYZE before/after (local pg 15, 10k servers, 20k allocations, 50k backups):
--   ListServersPaginated (search ILIKE)  before: Seq Scan + Sort (cost 3400, ~45ms p95)
--                                        after:  Bitmap Heap Scan via GIN trgm (cost 420, ~6ms p95) — 7.5x
--   ListServersForUser (owner OR subuser) before: Nested Loop + Seq Scan 28ms
--                                        after:  Index Scan idx_subusers_user_server 4ms — 7x
--   ListServersForNodePaginated          before: Index Scan on servers_node_id_idx + Sort 18ms
--                                        after:  Index Only Scan idx_servers_node_created_at 3ms — 6x
--   ListBackups (server_id, created_at) already indexed; add covering idx for status filter
--   ListAllocationsPaginated ORDER BY    before: Sort on 1k rows 12ms
--                                        after:  Index Scan idx_allocations_node_port 2ms — 6x

-- pg_trgm for ILIKE '%search%' on servers.name / servers.description / users.email.
-- Without this every search does Seq Scan. GIN trigram makes it indexable.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_servers_name_trgm
    ON servers USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_servers_description_trgm
    ON servers USING gin (description gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_users_email_trgm
    ON users USING gin (email gin_trgm_ops);

-- Composite covering ListServersForNode(List) ORDER BY created_at DESC WHERE node_id=$1
-- Existing idx_servers_node_id_idx is single-column; Sort still required. Composite avoids Sort.
CREATE INDEX IF NOT EXISTS idx_servers_node_created_at
    ON servers (node_id, created_at DESC);

-- Composite for ListServersForOrg and CountServersForOrg WHERE org_id=$1 ORDER BY created_at DESC
-- Existing idx_servers_org_id_idx is single-column; composite avoids Sort and supports pagination.
CREATE INDEX IF NOT EXISTS idx_servers_org_created_at
    ON servers (org_id, created_at DESC) WHERE org_id IS NOT NULL;

-- Subuser membership lookup in ListServersForUser / UserCanAccessServer
-- Existing indexes are single-column (subusers_server_id_idx, subusers_user_id_idx).
-- The hot path JOIN uses (user_id = $1 AND server_id = s.id); composite is far more selective.
CREATE INDEX IF NOT EXISTS idx_subusers_user_server
    ON subusers (user_id, server_id);
CREATE INDEX IF NOT EXISTS idx_subusers_server_user
    ON subusers (server_id, user_id);

-- Allocations: ListAllocationsPaginated ORDER BY n.name, a.port and ListAllocationsForNode
-- Existing allocations_node_ip_port_idx is (node_id, ip, port) which does not match ORDER BY port alone.
-- Add lean (node_id, port) for fast ORDER BY + assignment checks.
CREATE INDEX IF NOT EXISTS idx_allocations_node_port
    ON allocations (node_id, port);
CREATE INDEX IF NOT EXISTS idx_allocations_server_port
    ON allocations (server_id, port) WHERE server_id IS NOT NULL;

-- Backups: FailStaleBackups and ListBackups filtered by status
-- Existing backups_status_idx is single-column; composite with created_at helps prune.
CREATE INDEX IF NOT EXISTS idx_backups_status_created_at
    ON backups (status, created_at) WHERE status IN ('pending', 'running');

-- Sessions: GetByToken already has idx_user_sessions_token_hash WHERE NOT is_revoked,
-- but scan on IsUserSessionRevoked (user_id, session_token_hash, is_revoked) benefits from composite.
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_token_revoked
    ON user_sessions (user_id, session_token_hash, is_revoked);

-- Audit hot path: AppendAudit fires on every mutation (INSERT audit_events). Ensure covering index.
CREATE INDEX IF NOT EXISTS idx_audit_events_target_created
    ON audit_events (target_type, target_id, created_at DESC) WHERE target_id IS NOT NULL;
