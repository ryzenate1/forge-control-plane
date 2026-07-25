CREATE TABLE IF NOT EXISTS reconcile_plans (
    id uuid PRIMARY KEY,
    resource_id text NOT NULL,
    resource_kind text NOT NULL,
    state text NOT NULL DEFAULT 'pending',
    destructive bool NOT NULL DEFAULT false,
    confirmed bool NOT NULL DEFAULT false,
    diff_count int NOT NULL DEFAULT 0,
    drift_count int NOT NULL DEFAULT 0,
    diff_data jsonb NOT NULL DEFAULT '[]',
    drift_data jsonb NOT NULL DEFAULT '[]',
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    executed_at timestamptz,
    expires_at timestamptz,
    CONSTRAINT reconcile_plans_state_check CHECK (state IN
      ('pending','confirmed','executing','succeeded','failed','cancelled'))
);

CREATE TABLE IF NOT EXISTS reconcile_events (
    id uuid PRIMARY KEY,
    plan_id text NOT NULL,
    resource_id text NOT NULL,
    resource_kind text NOT NULL,
    event_type text NOT NULL,
    summary text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reconcile_plans_resource
    ON reconcile_plans(resource_id, resource_kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_state
    ON reconcile_plans(state, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_destructive
    ON reconcile_plans(destructive) WHERE destructive = true AND state = 'pending';
CREATE INDEX IF NOT EXISTS idx_reconcile_events_resource
    ON reconcile_events(resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reconcile_events_plan
    ON reconcile_events(plan_id, created_at DESC);
