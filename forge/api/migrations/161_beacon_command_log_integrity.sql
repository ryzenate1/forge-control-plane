-- Command IDs are idempotency keys. Keep the newest historical row if an
-- installation predating this constraint contains duplicates, then enforce the
-- invariant used by status updates.
DELETE FROM beacon_command_logs
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY command_id
                   ORDER BY created_at DESC, id DESC
               ) AS duplicate_rank
        FROM beacon_command_logs
    ) ranked
    WHERE duplicate_rank > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_beacon_logs_command_unique
    ON beacon_command_logs(command_id);

CREATE INDEX IF NOT EXISTS idx_beacon_logs_status_created
    ON beacon_command_logs(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_beacon_logs_correlation_created
    ON beacon_command_logs(correlation_id, created_at DESC);
