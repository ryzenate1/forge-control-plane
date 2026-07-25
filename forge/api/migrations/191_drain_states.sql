-- Phase 6: durable drain progress. The in-memory drain map in
-- clustermembership loses progress on restart; this table persists per-node
-- drain lifecycle (status, plan, terminal flags) plus a JSONB progress ledger
-- written by the drain driver (internal/services/drain).
CREATE TABLE IF NOT EXISTS drain_states (
    node_id       UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    plan_id       TEXT,
    status        TEXT NOT NULL DEFAULT 'draining',
    desired_final BOOLEAN NOT NULL DEFAULT false,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    progress      JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);