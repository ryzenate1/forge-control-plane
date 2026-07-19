ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS config_sync_pending BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS config_sync_error TEXT;

CREATE INDEX IF NOT EXISTS servers_config_sync_pending_idx
    ON servers (config_sync_pending)
    WHERE config_sync_pending = TRUE;

-- Transfer execution is intentionally retired. Do not leave legacy queued/running
-- metadata blocking lifecycle operations after upgrading.
UPDATE servers
SET transfer_state = 'failed',
    transfer_error = 'transfer execution is not implemented',
    transfer_target_node_id = NULL,
    transfer_run_token = NULL,
    transferring = FALSE,
    status = CASE WHEN status = 'transferring' THEN 'stopped' ELSE status END
WHERE transfer_state IN ('queued', 'running') OR transferring = TRUE;

CREATE TABLE IF NOT EXISTS server_orphan_remediations (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL,
    node_url TEXT NOT NULL,
    daemon_error TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS server_orphan_remediations_pending_idx
    ON server_orphan_remediations (created_at)
    WHERE status = 'pending';
