-- 235_schedule_runs_recovered_index.sql
-- Index backing the recovered_from_run_id self-FK added by 123 (and 044).
-- The FK column is written on lease recovery and read when tracing recovery
-- chains; without an index every such lookup is a sequential scan.
--
-- Additive and idempotent (IF NOT EXISTS). No backfill, no rewrite.

CREATE INDEX IF NOT EXISTS idx_schedule_runs_recovered_from_run_id
    ON schedule_runs (recovered_from_run_id);
