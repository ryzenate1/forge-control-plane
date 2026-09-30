-- 226_a: Docker disk-usage reporting & automated cleanup policies.
-- Inspired by Dokploy's docker-disk-usage router (getDiskUsage / getBuildCache /
-- pruneBuildCache) and CapRover's DiskCleanupManager (scheduled pruning of
-- unused images with a configurable retention count).
--
-- Named 226_a (not the plain 226 in the original brief) because 226_* is already
-- taken by 226_pipeline_runs_current_stage.sql and the migration runner rejects
-- duplicate numeric prefixes (see store.TestNoDuplicatePrefixesInInternalMigrations).
-- The 226_a letter suffix yields a distinct prefix while keeping the intended
-- ordering, exactly like 225_a_scheduled_tasks.sql sits beside 225_tags_and_mounts.sql.

-- A cleanup policy is either host-scoped (node_id set) or global (node_id NULL,
-- applied to every reachable node). `schedule` is a standard 5-field cron
-- expression. `most_recent_limit` is the retention floor: the scheduler always
-- preserves the N most recently created deployed images so a prune can never
-- strand the currently-running version.
CREATE TABLE IF NOT EXISTS docker_cleanup_policies (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id           UUID NULL REFERENCES nodes(id) ON DELETE CASCADE,
    schedule          VARCHAR(64) NOT NULL,
    most_recent_limit INT NOT NULL DEFAULT 1 CHECK (most_recent_limit >= 0),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    prune_build_cache BOOLEAN NOT NULL DEFAULT true,
    prune_volumes     BOOLEAN NOT NULL DEFAULT false,
    last_run_at       TIMESTAMPTZ NULL,
    next_run_at       TIMESTAMPTZ NULL,
    last_status       VARCHAR(32) NULL,
    last_error        TEXT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by        UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

-- Scheduler scan: "enabled policies whose next occurrence has arrived".
CREATE INDEX IF NOT EXISTS idx_docker_cleanup_policies_due
    ON docker_cleanup_policies (enabled, next_run_at);
-- Fast host-scoped lookups and the "at most one global policy" convenience scan.
CREATE INDEX IF NOT EXISTS idx_docker_cleanup_policies_node
    ON docker_cleanup_policies (node_id);

-- Store-based leader election: exactly one API replica drives the cleanup
-- scheduler loop at a time (TTL lease, self-renewing). Mirrors
-- scheduled_tasks_leader so the same lease semantics apply.
CREATE TABLE IF NOT EXISTS docker_cleanup_leader (
    name        TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
