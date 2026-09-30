-- No-op guard: this filename once held a byte-identical copy of 044_a_cloud_node_links.sql
-- (cloud_node_links table). The schema effect lives entirely in 044_a_cloud_node_links.sql.
--
-- This file is kept (not deleted, not renamed) because the filename is the
-- primary key in schema_migrations: hosts that already applied either name
-- must never see a "new" migration here. The runners treat the pair as
-- renames (see migrationAliases in internal/store/migration.go): if either
-- side applied, the other is recorded without re-running DDL.
SELECT 1;
