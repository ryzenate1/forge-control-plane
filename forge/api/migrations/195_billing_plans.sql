-- Phase 7 (Commerce): billing plans with JSON entitlements.
-- Plans are the source of truth for what an organization may create.
-- Columns keep PostgreSQL parity; JSONB collapses to TEXT on SQLite.
CREATE TABLE IF NOT EXISTS billing_plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    cents_per_month BIGINT NOT NULL DEFAULT 0,
    entitlements    JSONB NOT NULL DEFAULT '{}',
    trial_days      INTEGER NOT NULL DEFAULT 0,
    active          BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_billing_plans_active ON billing_plans (active);

-- Default catalog. The free plan is the fallback for every new org.
INSERT INTO billing_plans (code, name, cents_per_month, entitlements, trial_days, active) VALUES
    ('free', 'Free', 0,
        '{"max_servers": 2, "max_memory_gb": 4, "max_environments": 2, "max_nodes": 0, "price_per_gb": 0, "features": ["backups"]}',
        0, true),
    ('starter', 'Starter', 990,
        '{"max_servers": 10, "max_memory_gb": 32, "max_environments": 8, "max_nodes": 1, "price_per_gb": 250, "features": ["backups", "schedules", "metrics"]}',
        14, true),
    ('pro', 'Pro', 4900,
        '{"max_servers": 50, "max_memory_gb": 128, "max_environments": 32, "max_nodes": 4, "price_per_gb": 200, "features": ["backups", "schedules", "metrics", "vip"]}',
        0, true)
ON CONFLICT (code) DO NOTHING;