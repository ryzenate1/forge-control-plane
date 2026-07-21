CREATE TABLE IF NOT EXISTS install_workflows (
    id uuid PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    type text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    steps jsonb DEFAULT '[]',
    metadata jsonb DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_install_workflows_server ON install_workflows(server_id);
