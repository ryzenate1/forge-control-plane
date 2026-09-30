-- Pipeline run records (Phase 5). Status lifecycle:
-- queued -> running -> completed | failed | cancelled; awaiting_approval is a
-- paused running state reached when an approval stage is executed.
-- The stage_snapshot column freezes the resolved stage list at run creation
-- time so later edits to the definition do not mutate in-flight runs.
CREATE TABLE IF NOT EXISTS pipeline_runs (
    id                TEXT PRIMARY KEY,
    pipeline_id       TEXT NOT NULL REFERENCES pipeline_defs (id) ON DELETE CASCADE,
    trigger           TEXT NOT NULL DEFAULT 'manual',
    status            TEXT NOT NULL DEFAULT 'queued',
    stage_snapshot    JSONB NOT NULL DEFAULT '[]',
    progress_pct      INTEGER NOT NULL DEFAULT 0,
    current_stage_id  TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    retry_of          TEXT,
    retry_count       INTEGER NOT NULL DEFAULT 0,
    cancel_requested  BOOLEAN NOT NULL DEFAULT FALSE,
    requested_by      TEXT NOT NULL DEFAULT '',
    queued_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_runs_pipeline ON pipeline_runs (pipeline_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_status ON pipeline_runs (status, queued_at);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_retry_of ON pipeline_runs (retry_of);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_created_at ON pipeline_runs (created_at DESC);