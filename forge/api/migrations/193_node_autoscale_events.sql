-- Phase 6: node autoscaling audit ledger. Every worker evaluation records the
-- action taken (scale-out / scale-in / evaluate / error), its state
-- (suggested | applied | failed), the computed capacity deficit and a JSONB
-- detail blob (instance ids, node ids, load metrics).
CREATE TABLE IF NOT EXISTS node_autoscale_events (
    id         UUID PRIMARY KEY,
    policy_id  UUID REFERENCES node_autoscale_policies(id) ON DELETE CASCADE,
    action     TEXT NOT NULL,
    direction  TEXT NOT NULL DEFAULT 'out',
    state      TEXT NOT NULL DEFAULT 'suggested',
    deficit    INTEGER NOT NULL DEFAULT 0,
    detail     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);