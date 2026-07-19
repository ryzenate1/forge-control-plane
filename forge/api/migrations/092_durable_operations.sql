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
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    idempotency_key text,
    desired_generation bigint NOT NULL DEFAULT 1,
    observed_generation bigint NOT NULL DEFAULT 0,
    input jsonb NOT NULL DEFAULT '{}',
    result jsonb NOT NULL DEFAULT '{}',
    error text NOT NULL DEFAULT '',
    cancel_requested_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT operations_status_check CHECK (status IN
      ('queued','running','waiting','retrying','cancelling','rolling_back','succeeded','failed','cancelled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_operations_idempotency
    ON operations(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_operations_resource ON operations(resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_operations_status ON operations(status, created_at);

CREATE TABLE IF NOT EXISTS operation_steps (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    name text NOT NULL,
    position integer NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    max_attempts integer NOT NULL DEFAULT 3,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    UNIQUE(operation_id, position)
);

CREATE TABLE IF NOT EXISTS operation_attempts (
    id uuid PRIMARY KEY,
    operation_step_id uuid NOT NULL REFERENCES operation_steps(id) ON DELETE CASCADE,
    attempt integer NOT NULL,
    status text NOT NULL,
    worker_id text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(operation_step_id, attempt)
);
