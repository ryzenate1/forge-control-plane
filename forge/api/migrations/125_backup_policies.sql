CREATE TABLE IF NOT EXISTS backup_policies (
    id uuid PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    interval text NOT NULL,
    max_backups integer NOT NULL DEFAULT 10,
    retention_days integer NOT NULL DEFAULT 30,
    storage text NOT NULL DEFAULT 's3',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_backup_policies_server ON backup_policies(server_id);
