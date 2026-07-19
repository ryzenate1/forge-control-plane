ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS suspended BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS servers_suspended_idx ON servers (suspended);
