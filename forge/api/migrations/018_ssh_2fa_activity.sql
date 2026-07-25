-- No-op guard: this filename once held a byte-identical copy of 018_a_ssh_2fa_activity.sql
-- (SSH keys, 2FA and activity tables). The schema effect lives entirely in 018_a_ssh_2fa_activity.sql.
--
-- This file is kept (not deleted, not renamed) because the filename is the
-- primary key in schema_migrations: hosts that already applied either name
-- must never see a "new" migration here. The runners treat the pair as
-- renames (see migrationAliases in internal/store/migration.go): if either
-- side applied, the other is recorded without re-running DDL.
SELECT 1;
