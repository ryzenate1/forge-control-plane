-- Align the pipeline_runs column name with the store code.
--
-- Migration 186 created the column as `current_stage_id`, but the pipeline
-- Store (internal/services/pipeline/store.go) selects and updates
-- `current_stage` (GetRun/ListRuns SELECT, UpdateRunStatus SET). The mismatch
-- made GET /pipeline-runs fail with "column r.current_stage does not exist".
-- The value it holds is the current stage, so rename the column to match.
--
-- Idempotent + safe: only renames when the old name exists and the new one
-- does not. pipeline_runs has no cross-dialect copies (the pipeline phase is
-- Postgres-only, backed by *pgxpool.Pool).
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'pipeline_runs' AND column_name = 'current_stage_id'
  ) AND NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'pipeline_runs' AND column_name = 'current_stage'
  ) THEN
    ALTER TABLE pipeline_runs RENAME COLUMN current_stage_id TO current_stage;
  END IF;
END $$;
