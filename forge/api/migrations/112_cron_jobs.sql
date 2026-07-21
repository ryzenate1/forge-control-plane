CREATE TABLE IF NOT EXISTS cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    schedule VARCHAR(100) NOT NULL,
    command TEXT NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'shell',
    target_type VARCHAR(50),
    target_id UUID,
    enabled BOOLEAN NOT NULL DEFAULT true,
    retry_count INT NOT NULL DEFAULT 0,
    timeout_seconds INT NOT NULL DEFAULT 300,
    notify_on_failure BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cron_job_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cron_job_id UUID NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    exit_code INT,
    output TEXT,
    error TEXT,
    duration_ms INT
);

CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled);
CREATE INDEX IF NOT EXISTS idx_cron_job_executions_job_id ON cron_job_executions(cron_job_id);
CREATE INDEX IF NOT EXISTS idx_cron_job_executions_status ON cron_job_executions(status);
