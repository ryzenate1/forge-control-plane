CREATE TABLE IF NOT EXISTS procedures (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    tenant_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_procedures_tenant ON procedures(tenant_id);
CREATE INDEX IF NOT EXISTS idx_procedures_enabled ON procedures(enabled);

CREATE TABLE IF NOT EXISTS procedure_steps (
    id uuid PRIMARY KEY,
    procedure_id uuid NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    position integer NOT NULL,
    name text NOT NULL,
    action text NOT NULL,
    config jsonb NOT NULL DEFAULT '{}',
    max_retries integer NOT NULL DEFAULT 3,
    timeout_seconds integer NOT NULL DEFAULT 300,
    requires_approval boolean NOT NULL DEFAULT false,
    continue_on_failure boolean NOT NULL DEFAULT false,
    rollback_enabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(procedure_id, position)
);

CREATE TABLE IF NOT EXISTS procedure_executions (
    id uuid PRIMARY KEY,
    procedure_id uuid NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'queued',
    trigger text NOT NULL DEFAULT 'manual',
    tenant_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT procedure_executions_status_check CHECK (status IN
      ('queued','running','waiting_approval','succeeded','failed','cancelled','rolling_back','rolled_back'))
);

CREATE INDEX IF NOT EXISTS idx_procedure_executions_procedure ON procedure_executions(procedure_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_procedure_executions_status ON procedure_executions(status);
CREATE INDEX IF NOT EXISTS idx_procedure_executions_tenant ON procedure_executions(tenant_id);

CREATE TABLE IF NOT EXISTS procedure_step_executions (
    id uuid PRIMARY KEY,
    execution_id uuid NOT NULL REFERENCES procedure_executions(id) ON DELETE CASCADE,
    step_id uuid NOT NULL REFERENCES procedure_steps(id) ON DELETE CASCADE,
    position integer NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    attempt integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    output text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    started_at timestamptz,
    completed_at timestamptz,
    operation_id uuid REFERENCES operations(id) ON DELETE SET NULL,
    CONSTRAINT procedure_step_executions_status_check CHECK (status IN
      ('queued','running','succeeded','failed','skipped','waiting_approval','cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_procedure_step_execs_execution ON procedure_step_executions(execution_id, position);

CREATE TABLE IF NOT EXISTS procedure_schedules (
    id uuid PRIMARY KEY,
    procedure_id uuid NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    cron_expression text NOT NULL,
    timezone text NOT NULL DEFAULT 'UTC',
    enabled boolean NOT NULL DEFAULT true,
    last_run_at timestamptz,
    next_run_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(procedure_id)
);

CREATE INDEX IF NOT EXISTS idx_procedure_schedules_enabled ON procedure_schedules(enabled, next_run_at);

CREATE TABLE IF NOT EXISTS procedure_step_logs (
    id uuid PRIMARY KEY,
    step_execution_id uuid NOT NULL REFERENCES procedure_step_executions(id) ON DELETE CASCADE,
    level text NOT NULL DEFAULT 'info',
    message text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_procedure_step_logs_exec ON procedure_step_logs(step_execution_id, created_at);
