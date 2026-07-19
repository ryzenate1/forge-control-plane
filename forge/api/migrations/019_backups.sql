CREATE TABLE IF NOT EXISTS backups (
    uuid UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL DEFAULT '',
    size BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    upload_id TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, name)
);

CREATE INDEX IF NOT EXISTS backups_server_id_created_at_idx ON backups (server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS backups_status_idx ON backups (status);
