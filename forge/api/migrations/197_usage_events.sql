-- Phase 7 (Commerce): raw usage meter events.
-- Every RecordUsage call appends one event; the hourly reaper rolls the events
-- up into org_quotas counters and event rows are retained for 7 days.
CREATE TABLE IF NOT EXISTS usage_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    resource    TEXT NOT NULL,
    quantity    BIGINT NOT NULL DEFAULT 1,
    kind        TEXT NOT NULL DEFAULT 'meter',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_usage_events_org_time ON usage_events (org_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_usage_events_time ON usage_events (occurred_at);