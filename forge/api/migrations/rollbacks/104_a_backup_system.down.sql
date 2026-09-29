-- Backup & Recovery System - Rollback for 104_a_backup_system.sql
-- Drops all tables, views and functions created by that migration.
--
-- Filename matters: Store.Rollback (internal/store/store.go) computes the down
-- file as strings.TrimSuffix(version, ".sql") + ".down.sql" where `version` is
-- the canonical filename recorded in schema_migrations, i.e.
-- "104_a_backup_system.sql" (letter suffix included; confirmed against the live
-- dev database). This file previously shipped as "104_backup_system.down.sql",
-- which that lookup can never produce, so the migration reported as
-- non-reversible while a down file sat next to it.
--
-- backup_* tables (7) plus the four generic tables apps / databases / volumes /
-- encryption_keys are all created here and referenced by no other migration
-- (grep over migrations/*.sql) and queried by no Go code, so they are part of
-- exactly this undo.

-- Drop triggers first
DROP TRIGGER IF EXISTS backup_config_updated_at_trigger ON backup_configurations;
DROP TRIGGER IF EXISTS backup_job_updated_at_trigger ON backup_jobs;
DROP TRIGGER IF EXISTS backup_artifact_updated_at_trigger ON backup_artifacts;
DROP TRIGGER IF EXISTS backup_restore_updated_at_trigger ON backup_restores;
DROP TRIGGER IF EXISTS backup_verification_updated_at_trigger ON backup_verifications;
DROP TRIGGER IF EXISTS backup_retention_updated_at_trigger ON backup_retention_policies;
DROP TRIGGER IF EXISTS backup_storage_updated_at_trigger ON backup_storage_providers;

-- Drop functions
DROP FUNCTION IF EXISTS update_backup_config_updated_at();
DROP FUNCTION IF EXISTS update_backup_job_updated_at();
DROP FUNCTION IF EXISTS update_backup_artifact_updated_at();
DROP FUNCTION IF EXISTS update_backup_restore_updated_at();
DROP FUNCTION IF EXISTS update_backup_verification_updated_at();
DROP FUNCTION IF EXISTS update_backup_retention_updated_at();
DROP FUNCTION IF EXISTS update_backup_storage_updated_at();
DROP FUNCTION IF EXISTS calculate_next_cron_run(TEXT);

-- Drop views (before their base tables; they only select from backup_*)
DROP VIEW IF EXISTS backup_system_overview;
DROP VIEW IF EXISTS backup_statistics;

-- Drop tables (in reverse order of creation to handle foreign keys)
DROP TABLE IF EXISTS backup_storage_providers;
DROP TABLE IF EXISTS backup_retention_policies;
DROP TABLE IF EXISTS backup_verifications;
DROP TABLE IF EXISTS backup_restores;
DROP TABLE IF EXISTS backup_artifacts;
DROP TABLE IF EXISTS backup_jobs;
DROP TABLE IF EXISTS backup_configurations;
DROP TABLE IF EXISTS encryption_keys;
DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS apps;
