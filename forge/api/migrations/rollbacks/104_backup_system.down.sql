-- Backup & Recovery System - Rollback Migration
-- Drops all tables and views created in the up migration

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

-- Drop views
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
