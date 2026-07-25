ALTER TABLE deployments ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER NOT NULL DEFAULT 300;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS health_gate_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS health_gate_threshold INTEGER NOT NULL DEFAULT 3;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS health_gate_interval_ms INTEGER NOT NULL DEFAULT 5000;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS auto_rollback_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollback_on_health_failure BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS cleanup_on_failure BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS target_replicas INTEGER NOT NULL DEFAULT 1;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS progress_pct INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS next_step INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS timeout_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS deployment_steps (
    id UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    step_number INTEGER NOT NULL,
    step_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_steps_deployment ON deployment_steps(deployment_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_steps_step ON deployment_steps(deployment_id, step_number);
