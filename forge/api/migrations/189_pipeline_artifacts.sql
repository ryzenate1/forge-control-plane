-- Pipeline artifacts (Phase 5). Metadata for files produced by pipeline
-- stages. Files themselves are stored on the local filesystem under
-- data/pipelines/artifacts/<runId>/<stageId>/<name> (see the pipeline
-- service artifact handler). relative_path is store-relative so a future S3
-- backend can swap in without migrating rows.
CREATE TABLE IF NOT EXISTS pipeline_artifacts (
    id            TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL REFERENCES pipeline_runs (id) ON DELETE CASCADE,
    stage_id      TEXT,
    name          TEXT NOT NULL,
    relative_path TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    content_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_artifacts_run ON pipeline_artifacts (run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_pipeline_artifacts_stage ON pipeline_artifacts (stage_id);