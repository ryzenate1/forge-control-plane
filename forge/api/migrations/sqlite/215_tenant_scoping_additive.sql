-- 215_tenant_scoping_additive.sql (SQLite dialect)
-- Additive tenancy scoping for core paths: events, placement_decisions, reconcile_plans
-- SQLite uses TEXT for UUID; indexes are partial where SQLite supports.
ALTER TABLE events ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE placement_decisions ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant ON placement_decisions (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE reconcile_plans ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant ON reconcile_plans (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE reconcile_events ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_reconcile_events_tenant ON reconcile_events (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE placement_reservations ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_placement_reservations_tenant ON placement_reservations (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE placement_intents ADD COLUMN tenant_id TEXT;
CREATE INDEX IF NOT EXISTS idx_placement_intents_tenant ON placement_intents (tenant_id) WHERE tenant_id IS NOT NULL;
