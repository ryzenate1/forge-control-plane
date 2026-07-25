-- 214_forge_leader — advisory-lock/TTL leader election for maintenance daemons (AF-2)
-- Reuses queue/leader.go elector logic. The table is separate from river_leader so that
-- queue and legacy river do not contend; it gates reconciler, periodic scheduler,
-- retention pruners, and failover timers to a single elected replica.

CREATE TABLE IF NOT EXISTS forge_leader (
    id TEXT PRIMARY KEY DEFAULT 'forge',
    leader_id TEXT NOT NULL,
    elected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT forge_leader_singleton CHECK (id = 'forge')
);

CREATE INDEX IF NOT EXISTS idx_forge_leader_expires ON forge_leader(expires_at);

-- Helper: attempt to elect or re-elect. TTL semantics identical to queue/leader.go.
--   Elect: INSERT ... ON CONFLICT(id) DO UPDATE WHERE expires_at < NOW() OR leader_id = EXCLUDED.leader_id
-- Called via pgx as LeaderAttemptElect / LeaderAttemptReelect through a minimal executor
-- that wraps this table (see forge/api/internal/leader/elector.go). The SQL itself lives
-- in the application layer to keep the TTL tunable via QUEUE_LEADER_TTL env.

COMMENT ON TABLE forge_leader IS 'Singleton leader election for forge maintenance daemons (AF-2). Only the elected replica runs reconciler/periodic/retention/failover.';
