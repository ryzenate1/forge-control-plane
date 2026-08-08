-- Structured stage execution logs (Phase 5). Every executor appends rows for
-- progress, action output and failures; the id column doubles as the ordering
-- cursor for live SSE/WebSocket log streams.
CREATE TABLE IF NOT EXISTS pipeline_stage_logs (
    id         BIGSERIAL PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES pipeline_runs (id) ON DELETE CASCADE,
    stage_id   TEXT NOT NULL REFERENCES pipeline_stage_runs (id) ON DELETE CASCADE,
    level      TEXT NOT NULL DEFAULT 'info',
    message    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_stage_logs_run ON pipeline_stage_logs (run_id, id);
CREATE INDEX IF NOT EXISTS idx_pipeline_stage_logs_stage ON pipeline_stage_logs (stage_id, id);