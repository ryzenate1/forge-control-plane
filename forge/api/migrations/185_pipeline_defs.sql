-- Reusable CI/CD pipeline definitions (Phase 5). A pipeline def is an
-- ordered set of stages, each stage being a single action
-- (pull/build/deploy/compose/health_check/notify/approval/sleep/script) with
-- per-stage config, a timeout, and a retry policy.
CREATE TABLE IF NOT EXISTS pipeline_defs (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    categories  TEXT[] NOT NULL DEFAULT '{}',
    stages      JSONB NOT NULL DEFAULT '[]',
    trigger     JSONB NOT NULL DEFAULT '{"type":"manual"}',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_defs_name ON pipeline_defs (name);
CREATE INDEX IF NOT EXISTS idx_pipeline_defs_categories ON pipeline_defs USING GIN (categories);