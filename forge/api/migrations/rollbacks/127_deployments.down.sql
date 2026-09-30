-- Rollback for 127_deployments.sql.
--
-- 127 does NOT create the deployments table: 082_deployments.sql and
-- 095_deployments.sql do, and 086 / 099 / 100 / 103_a / 104_b / 105 / 107 ALTER
-- it afterwards. This file previously read `DROP TABLE IF EXISTS deployments;`,
-- which destroyed a table owned by other migrations (with every deployment row)
-- while removing only 127's own ledger entry.
--
-- Exact undo of 127's own objects (all three verified present on the live dev
-- database, and created by no other migration file):
DROP INDEX IF EXISTS idx_deployments_status;
DROP INDEX IF EXISTS idx_deployments_strategy;
DROP INDEX IF EXISTS idx_deployments_created_at;
--
-- fk_deployments_server is deliberately NOT dropped here. 086_add_table_constraints.sql
-- creates that constraint first on every install (deployments exists from 082, which
-- is before 086), so 127's DO $$ ... duplicate_object $$ re-add is always the no-op
-- side of a shared object. Dropping it here would delete an object that
-- 086_add_table_constraints.sql still claims in schema_migrations; rolling 086 back is
-- a separate concern and 086 ships no down file.
