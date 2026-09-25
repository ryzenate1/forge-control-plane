-- 225: per-app scheduled tasks (Dokku cron / Dokploy schedule style).
-- Users define cron-expression commands that run inside their application's
-- container context; the API scheduler dispatches execution to Beacon and
-- records every run for history. Named 225 (not 222 as originally planned)
-- because 222_* is already taken by scheduler_policies and the duplicate
-- prefix validator rejects new collisions.
CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    command     TEXT NOT NULL,
    schedule    VARCHAR(64) NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ NULL,
    next_run_at TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

-- Scheduler scan: "enabled tasks whose next occurrence has arrived".
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_due ON scheduled_tasks (enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_server ON scheduled_tasks (server_id);

CREATE TABLE IF NOT EXISTS task_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id     UUID NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ NULL,
    exit_code   INT NULL,
    output      TEXT NOT NULL DEFAULT '',
    status      VARCHAR(32) NOT NULL DEFAULT 'running'
);

CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs (task_id, started_at DESC);

-- Store-based leader election: exactly one API instance drives the
-- scheduled-task scheduler loop at a time (TTL lease, self-renewing).
CREATE TABLE IF NOT EXISTS scheduled_tasks_leader (
    name        TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);
