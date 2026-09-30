-- Stale running operation reaper: operations stuck in 'running' due to worker crash
-- need efficient lookup on updated_at for the reaper's 5m threshold.
CREATE INDEX IF NOT EXISTS idx_operations_running_updated_at
    ON operations(status, updated_at) WHERE status = 'running';
-- Alternative locked_until-style lease index (if locked_until column is added later):
-- CREATE INDEX IF NOT EXISTS idx_operations_locked_until ON operations(locked_until) WHERE status = 'running';
-- Keep updated_at index as primary for current implementation.
