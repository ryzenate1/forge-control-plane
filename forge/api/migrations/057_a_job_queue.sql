CREATE TABLE IF NOT EXISTS job_queue (
    id uuid PRIMARY KEY,
    type text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    server_id uuid,
    node_id uuid,
    payload jsonb DEFAULT '{}',
    result jsonb DEFAULT '{}',
    error text DEFAULT '',
    priority integer NOT NULL DEFAULT 0,
    max_retries integer NOT NULL DEFAULT 3,
    retry_count integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_job_queue_status ON job_queue(status, priority, created_at);
CREATE INDEX IF NOT EXISTS idx_job_queue_node ON job_queue(node_id, status);
