-- Stale running operation reaper for SQLite: index on running operations by updated_at
CREATE INDEX IF NOT EXISTS idx_operations_running_updated_at
    ON operations(status, updated_at) WHERE status = 'running';
