-- Per-run stage execution records (Phase 5). Generated at run creation from
-- the frozen definition snapshot; the executor advances them in order.
CREATE TABLE IF NOT EXISTS pipeline_stage_runs (
    id            TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL REFERENCES pipeline_runs (id) ON DELETE CASCADE,
    pipeline_id   TEXT NOT NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    name          TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL DEFAULT '',
    config        JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'queued',
    attempts      INTEGER NOT NULL DEFAULT 0,
    timeout_sec   INTEGER NOT NULL DEFAULT 300,
    retry_max     INTEGER NOT NULL DEFAULT 0,
    retry_backoff_ms  INTEGER NOT NULL DEFAULT 0,
    retry_max_sleep_ms INTEGER NOT NULL DEFAULT 0,
    continue_on_failure BOOLEAN NOT NULL DEFAULT FALSE,
    error         TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_stage_runs_run ON pipeline_stage_runs (run_id, position);
CREATE INDEX IF NOT EXISTS idx_pipeline_stage_runs_status ON pipeline_stage_runs (status);