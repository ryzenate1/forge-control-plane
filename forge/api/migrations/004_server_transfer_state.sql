ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS transferring BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS transfer_target_node_id UUID REFERENCES nodes(id),
    ADD COLUMN IF NOT EXISTS transfer_requested_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS servers_transferring_idx ON servers (transferring);
