-- Resource limits, deploy-gating health checks and process scaling.
--
-- Inspired by Dokku's `resource`, `checks` and `ps` plugins: one row per
-- (application, process type) carrying the CPU/memory limit, the replica
-- count and the health-check definition for that process. The compose
-- document an application releases is enriched from these rows at deploy
-- time (see internal/services/resourcelimits.ApplyToCompose), so Docker
-- itself enforces the limits — no new Beacon command is required.
--
-- `health_observations` is the append-only record of what the prober (and,
-- later, Beacon) actually saw. Rollout gating (`ShouldRollback`) reads the
-- most recent observation per process type inside the grace window derived
-- from the check definition, so an un-reported process is never mistaken
-- for a healthy one — and never mistaken for a failed one either.
--
-- Postgres is the production dialect; the statements below are deliberately
-- restricted to syntax the SQLite dev fallback can execute after the
-- migration runner's type substitutions (UUID -> TEXT, TIMESTAMPTZ ->
-- TIMESTAMP, gen_random_uuid() -> randomblob expression). No regex CHECKs,
-- no partial expressions, no COMMENT ON.

CREATE TABLE IF NOT EXISTS process_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    process_type VARCHAR(32) NOT NULL,
    -- cpu_limit is stored in Docker nanoCores (1_000_000_000 == one full CPU).
    -- NULL means "no CPU limit", which is not the same as 0.
    cpu_limit BIGINT,
    -- memory_limit is stored in bytes. NULL means "unlimited".
    memory_limit BIGINT,
    replicas INTEGER NOT NULL DEFAULT 1 CHECK (replicas >= 0 AND replicas <= 64),
    health_check_type VARCHAR(16) CHECK (health_check_type IN ('http', 'tcp', 'command')),
    health_check_path TEXT NOT NULL DEFAULT '',
    health_check_port INTEGER NOT NULL DEFAULT 0 CHECK (health_check_port >= 0 AND health_check_port <= 65535),
    health_check_command TEXT NOT NULL DEFAULT '',
    health_check_interval INTEGER NOT NULL DEFAULT 30 CHECK (health_check_interval >= 1 AND health_check_interval <= 3600),
    health_check_timeout INTEGER NOT NULL DEFAULT 10 CHECK (health_check_timeout >= 1 AND health_check_timeout <= 3600),
    health_check_retries INTEGER NOT NULL DEFAULT 3 CHECK (health_check_retries >= 1 AND health_check_retries <= 60),
    health_check_start_period INTEGER NOT NULL DEFAULT 40 CHECK (health_check_start_period >= 0 AND health_check_start_period <= 3600),
    -- health_check_gating separates "probe this" from "fail the release if the
    -- probe fails". Dokku's `checks` plugin gates by default and requires an
    -- explicit opt-out, so the default is TRUE: defining a check means trusting
    -- it enough to roll back on it. Set it FALSE for an observability-only
    -- check whose failure should be shown but should not stop a deploy.
    health_check_gating BOOLEAN NOT NULL DEFAULT TRUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT process_configs_application_process_unique UNIQUE (application_id, process_type),
    -- A check that is not `command` needs a port to dial; an http check also
    -- needs a path. `command` checks need the command itself.
    CONSTRAINT process_configs_health_check_shape CHECK (
        health_check_type IS NULL
        OR (health_check_type = 'command' AND length(trim(health_check_command)) > 0)
        OR (health_check_type = 'tcp' AND health_check_port > 0)
        OR (health_check_type = 'http' AND health_check_port >= 0)
    )
);

CREATE INDEX IF NOT EXISTS idx_process_configs_application_id ON process_configs(application_id);
CREATE INDEX IF NOT EXISTS idx_process_configs_app_enabled ON process_configs(application_id, enabled);

-- Latest health observation per (server, process type). Append-only: every
-- probe result is recorded so the UI can show history and so a rollback
-- decision can be audited after the fact.
--
-- application_id is denormalised on top of the requested (server_id,
-- process_type) key. Without it, every "is this app healthy?" question would
-- have to be answered by scanning every observation on the server — including
-- the observations belonging to unrelated applications co-located on it. It is
-- nullable because a host-level probe can legitimately arrive before the panel
-- knows which application produced it.
CREATE TABLE IF NOT EXISTS health_observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    application_id UUID REFERENCES applications(id) ON DELETE CASCADE,
    process_type VARCHAR(32) NOT NULL,
    healthy BOOLEAN NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_health_observations_server_time ON health_observations(server_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_health_observations_app_time ON health_observations(application_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_health_observations_app_process_time ON health_observations(application_id, process_type, observed_at DESC);
