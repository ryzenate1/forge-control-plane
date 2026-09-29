-- 235_schedule_runs_recovered_index.sql (MySQL dialect)
-- MySQL has no CREATE INDEX IF NOT EXISTS; the runner executes each file
-- once (recorded in schema_migrations), so plain CREATE INDEX converges.
-- On a best-effort MySQL retry after a partial apply, a duplicate-index
-- error means the index already exists — drop it manually or ignore.
CREATE INDEX idx_schedule_runs_recovered_from_run_id ON schedule_runs (recovered_from_run_id);
