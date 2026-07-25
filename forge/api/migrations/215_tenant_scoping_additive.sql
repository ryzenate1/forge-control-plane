-- 215_tenant_scoping_additive.sql
-- Additive tenancy scoping for core paths: events, placement_decisions, reconcile_plans
-- All columns nullable UUID, no breaking change, backfill async.
-- Tenant maps to organizations(id) logically; FK deferred to avoid blocking DDL on large tables.
-- Indexes are partial WHERE tenant_id IS NOT NULL so they cost nothing until populated.

-- events outbox (primary tenant partition for audit)
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id UUID;
ALTER TABLE events_dead_letter ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_tenant_type ON events (tenant_id, type) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_dl_tenant ON events_dead_letter (tenant_id) WHERE tenant_id IS NOT NULL;

-- placement decisions (replica placement tenant isolation)
ALTER TABLE placement_decisions ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant ON placement_decisions (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant_app ON placement_decisions (tenant_id, app_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant_node ON placement_decisions (tenant_id, node_id) WHERE tenant_id IS NOT NULL;

-- reconcile plans (fleet-wide drift/equality planning tenant partition)
ALTER TABLE reconcile_plans ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant ON reconcile_plans (tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant_state ON reconcile_plans (tenant_id, state) WHERE tenant_id IS NOT NULL;

ALTER TABLE reconcile_events ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_reconcile_events_tenant ON reconcile_events (tenant_id) WHERE tenant_id IS NOT NULL;

-- placement reservations / intents also tenant-scoped (additive, non-breaking)
ALTER TABLE placement_reservations ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_placement_reservations_tenant ON placement_reservations (tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE placement_intents ADD COLUMN IF NOT EXISTS tenant_id UUID;
CREATE INDEX IF NOT EXISTS idx_placement_intents_tenant ON placement_intents (tenant_id) WHERE tenant_id IS NOT NULL;
