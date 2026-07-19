ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS transfer_state TEXT NOT NULL DEFAULT 'idle',
    ADD COLUMN IF NOT EXISTS transfer_error TEXT;

CREATE INDEX IF NOT EXISTS servers_transfer_state_idx ON servers (transfer_state);
