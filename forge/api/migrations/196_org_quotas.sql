-- Phase 7 (Commerce): per-organization quota rows.
-- One row per org; plan_code mirrors billing_plans.code. Live counters are a
-- cached projection refreshed by the hourly usage reaper (and on read when
-- stale) so create paths can keep using plain deletes/inserts.
CREATE TABLE IF NOT EXISTS org_quotas (
    org_id              UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    plan_code           TEXT NOT NULL DEFAULT 'free',
    trial_until         TIMESTAMPTZ,
    memory_usage_bytes  BIGINT NOT NULL DEFAULT 0,
    servers_count       BIGINT NOT NULL DEFAULT 0,
    environments_count  BIGINT NOT NULL DEFAULT 0,
    nodes_count         BIGINT NOT NULL DEFAULT 0,
    storage_bytes       BIGINT NOT NULL DEFAULT 0,
    prev_plan_code      TEXT NOT NULL DEFAULT '',
    quota_updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_org_quotas_plan ON org_quotas (plan_code);