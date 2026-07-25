ALTER TABLE operations
    ADD COLUMN IF NOT EXISTS input JSONB NOT NULL DEFAULT '{}'::JSONB,
    ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS operation_steps (
    id TEXT PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'retrying', 'succeeded', 'failed', 'cancelled')),
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (operation_id, position)
);

CREATE INDEX IF NOT EXISTS operation_steps_operation_idx
    ON operation_steps (operation_id, position);

CREATE TABLE IF NOT EXISTS operation_attempts (
    id TEXT PRIMARY KEY,
    operation_step_id TEXT NOT NULL REFERENCES operation_steps(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    worker_id TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE (operation_step_id, attempt)
);

CREATE INDEX IF NOT EXISTS operation_attempts_step_idx
    ON operation_attempts (operation_step_id, attempt);
