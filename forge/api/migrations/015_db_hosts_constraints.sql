-- No-op guard: this filename once held a byte-identical copy of 120_a_db_hosts_constraints.sql
-- (database_hosts name/node unique index). The schema effect lives entirely in 120_a_db_hosts_constraints.sql.
--
-- This file is kept (not deleted, not renamed) because the filename is the
-- primary key in schema_migrations: hosts that already applied either name
-- must never see a "new" migration here. The runners treat the pair as
-- renames (see migrationAliases in internal/store/migration.go): if either
-- side applied, the other is recorded without re-running DDL.
SELECT 1;
