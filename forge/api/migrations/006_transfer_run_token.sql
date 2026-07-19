ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS transfer_run_token UUID;

CREATE INDEX IF NOT EXISTS servers_transfer_run_token_idx ON servers (transfer_run_token);
