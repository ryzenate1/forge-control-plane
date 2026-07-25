-- 215_tenant_scoping_additive.sql (MySQL/MariaDB dialect)
--
-- Mirror of the canonical set of tenant_id columns and their supporting
-- indexes. Two things the previous override got wrong are corrected here:
--   * canonical also adds tenant_id to events_dead_letter and four composite
--     indexes (events(tenant_id,type), placement_decisions(tenant_id,app_id),
--     placement_decisions(tenant_id,node_id), reconcile_plans(tenant_id,state));
--     the override omitted them, so MySQL hosts had no tenant scoping on the
--     dead-letter outbox at all.
--   * canonical indexes are partial (WHERE tenant_id IS NOT NULL) which
--     MySQL/MariaDB cannot express, so they are plain indexes here; every
--     non-partial equivalent is kept so query plans still have an index.
-- ADD COLUMN / CREATE INDEX IF NOT EXISTS (MariaDB spelling) are required, not
-- cosmetic: MySQL commits DDL implicitly, so a mid-file failure leaves earlier
-- statements committed with no schema_migrations row and the retry MUST converge.
ALTER TABLE events ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
ALTER TABLE events_dead_letter ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_events_tenant ON events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_events_tenant_type ON events (tenant_id, type);
CREATE INDEX IF NOT EXISTS idx_events_dl_tenant ON events_dead_letter (tenant_id);

-- placement decisions (replica placement tenant isolation)
ALTER TABLE placement_decisions ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant ON placement_decisions (tenant_id);
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant_app ON placement_decisions (tenant_id, app_id);
CREATE INDEX IF NOT EXISTS idx_placement_decisions_tenant_node ON placement_decisions (tenant_id, node_id);

-- reconcile plans (fleet-wide drift/equality planning tenant partition)
ALTER TABLE reconcile_plans ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant ON reconcile_plans (tenant_id);
CREATE INDEX IF NOT EXISTS idx_reconcile_plans_tenant_state ON reconcile_plans (tenant_id, state);

ALTER TABLE reconcile_events ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_reconcile_events_tenant ON reconcile_events (tenant_id);

-- placement reservations / intents also tenant-scoped (additive, non-breaking)
ALTER TABLE placement_reservations ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_placement_reservations_tenant ON placement_reservations (tenant_id);

ALTER TABLE placement_intents ADD COLUMN IF NOT EXISTS tenant_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_placement_intents_tenant ON placement_intents (tenant_id);
