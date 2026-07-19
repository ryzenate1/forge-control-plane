-- Durable delivery outboxes and schedule execution leases.

CREATE TABLE IF NOT EXISTS mail_outbox (
    id UUID PRIMARY KEY,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    text_body TEXT NOT NULL DEFAULT '',
    html_body TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mail_outbox_pending_idx
    ON mail_outbox (next_attempt_at, created_at)
    WHERE sent_at IS NULL;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY,
    webhook_id TEXT REFERENCES webhooks(id) ON DELETE SET NULL,
    event_name TEXT NOT NULL,
    target_url TEXT NOT NULL,
    webhook_type TEXT NOT NULL CHECK (webhook_type IN ('regular', 'discord')),
    secret TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    request_body JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    response_status INTEGER,
    response_body_excerpt TEXT NOT NULL DEFAULT '',
    last_error TEXT,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'processing', 'delivered', 'failed')),
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_pending_idx
    ON webhook_deliveries (next_attempt_at, created_at)
    WHERE state IN ('pending', 'processing');
CREATE INDEX IF NOT EXISTS webhook_deliveries_history_idx
    ON webhook_deliveries (webhook_id, created_at DESC);

ALTER TABLE server_schedules
    ADD COLUMN IF NOT EXISTS lease_owner TEXT,
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS server_schedules_due_lease_idx
    ON server_schedules (next_run_at, lease_expires_at)
    WHERE enabled = TRUE;

ALTER TABLE schedule_runs
    ADD COLUMN IF NOT EXISTS worker_id TEXT,
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS recovered_from_run_id UUID REFERENCES schedule_runs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS schedule_runs_running_lease_idx
    ON schedule_runs (lease_expires_at)
    WHERE status = 'running';
