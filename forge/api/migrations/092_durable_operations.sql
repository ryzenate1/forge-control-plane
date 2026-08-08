ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS idempotency_key text;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS available_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS locked_by text;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS locked_until timestamptz;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS last_heartbeat_at timestamptz;

ALTER TABLE servers ADD COLUMN IF NOT EXISTS desired_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS observed_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS last_observation_at timestamptz;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS last_reconcile_error text NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS desired_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS observed_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_observation_at timestamptz;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_reconcile_error text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_job_queue_idempotency
    ON job_queue(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_job_queue_available
    ON job_queue(status, available_at, priority DESC);
CREATE INDEX IF NOT EXISTS idx_job_queue_expired_lease
    ON job_queue(locked_until) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS operations (
    id uuid PRIMARY KEY,
    kind text NOT NULL,
    resource_type text NOT NULL CHECK (resource_type IN ('server', 'node', 'allocation', 'backup', 'deployment', 'user', 'database', 'volume', 'app', 'compose')),
    resource_id text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    idempotency_key text,
    desired_generation bigint NOT NULL DEFAULT 1,
    observed_generation bigint NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT operations_status_check CHECK (status IN ('queued','running','waiting','retrying','cancelling','rolling_back','succeeded','failed','cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_operations_resource
    ON operations(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_operations_status
    ON operations(status);
CREATE INDEX IF NOT EXISTS idx_operations_idempotency
    ON operations(idempotency_key) WHERE idempotency_key IS NOT NULL;
