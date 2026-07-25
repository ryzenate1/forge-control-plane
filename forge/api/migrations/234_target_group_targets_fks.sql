-- Add the missing foreign keys on target_group_targets.server_id / node_id.
-- Both 082 variants created these columns as TEXT without references, while
-- servers(id) and nodes(id) are UUID, so the columns are converted first and
-- then constrained. Uses DO blocks for idempotency across supported
-- PostgreSQL versions (same pattern as 086_add_table_constraints.sql).
--
-- Failure semantics are loud by design: if a legacy row holds a non-UUID
-- value, the ALTER ... USING ::uuid cast errors instead of silently leaving
-- the table unconstrained. Fix the data and re-run; the migration is
-- idempotent (converting UUID->UUID is a no-op, constraints are IF-guarded).
--
-- Dialects: MySQL is best-effort and needs a mysql/ override for this file
-- (ALTER ... TYPE has no MySQL spelling and the translator does not cover
-- it). On SQLite the ALTER/ADD CONSTRAINT statements are skipped per the
-- documented sqliteSkippedDDL gap while the indexes below still apply.

-- 1) server_id: TEXT -> UUID to match servers(id), then FK.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'target_group_targets') THEN
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'target_group_targets'
              AND column_name = 'server_id'
              AND udt_name <> 'uuid'
        ) THEN
            EXECUTE 'ALTER TABLE target_group_targets ALTER COLUMN server_id TYPE UUID USING server_id::uuid';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_target_group_targets_server') THEN
            EXECUTE 'ALTER TABLE target_group_targets
                ADD CONSTRAINT fk_target_group_targets_server
                FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE';
        END IF;
    END IF;
END $$;

-- 2) node_id: TEXT -> UUID to match nodes(id), then FK.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'target_group_targets') THEN
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'target_group_targets'
              AND column_name = 'node_id'
              AND udt_name <> 'uuid'
        ) THEN
            EXECUTE 'ALTER TABLE target_group_targets ALTER COLUMN node_id TYPE UUID USING node_id::uuid';
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_target_group_targets_node') THEN
            EXECUTE 'ALTER TABLE target_group_targets
                ADD CONSTRAINT fk_target_group_targets_node
                FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE';
        END IF;
    END IF;
END $$;

-- 3) Indexes backing the new foreign keys.
CREATE INDEX IF NOT EXISTS idx_target_group_targets_server_id ON target_group_targets (server_id);
CREATE INDEX IF NOT EXISTS idx_target_group_targets_node_id ON target_group_targets (node_id);
