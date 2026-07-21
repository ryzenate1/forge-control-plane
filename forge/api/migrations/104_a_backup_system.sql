-- Backup & Recovery System - Complete Implementation
-- This migration creates a comprehensive backup system with configurations, jobs, artifacts, and restores

-- Stub tables for foreign key references
CREATE TABLE IF NOT EXISTS apps (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS databases (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    server_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS volumes (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS encryption_keys (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    algorithm TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backup configurations table (for scheduled backups)
CREATE TABLE IF NOT EXISTS backup_configurations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    database_id UUID REFERENCES databases(id) ON DELETE CASCADE,
    volume_id UUID REFERENCES volumes(id) ON DELETE CASCADE,
    
    -- Backup type configuration
    backup_type VARCHAR(50) NOT NULL CHECK (backup_type IN ('app', 'volume', 'database', 'server')),
    
    -- Schedule configuration
    is_scheduled BOOLEAN NOT NULL DEFAULT FALSE,
    cron_expression VARCHAR(100),
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    
    -- Storage configuration
    storage_provider VARCHAR(50) NOT NULL DEFAULT 'local',
    storage_config JSONB,
    
    -- Retention policy
    max_backups INTEGER NOT NULL DEFAULT 10,
    retention_days INTEGER NOT NULL DEFAULT 30,
    
    -- Backup settings
    compression_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    encryption_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    encryption_key_id UUID REFERENCES encryption_keys(id) ON DELETE SET NULL,
    
    -- Status
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_status VARCHAR(50),
    last_error TEXT,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT backup_config_unique_name UNIQUE (name),
    CONSTRAINT backup_config_server_or_app_or_db CHECK (
        (server_id IS NOT NULL AND app_id IS NULL AND database_id IS NULL AND volume_id IS NULL) OR
        (server_id IS NULL AND app_id IS NOT NULL AND database_id IS NULL AND volume_id IS NULL) OR
        (server_id IS NULL AND app_id IS NULL AND database_id IS NOT NULL AND volume_id IS NULL) OR
        (server_id IS NULL AND app_id IS NULL AND database_id IS NULL AND volume_id IS NOT NULL)
    )
);

-- Indexes for backup_configurations
CREATE INDEX IF NOT EXISTS idx_backup_configurations_server ON backup_configurations(server_id);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_app ON backup_configurations(app_id);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_database ON backup_configurations(database_id);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_volume ON backup_configurations(volume_id);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_type ON backup_configurations(backup_type);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_enabled ON backup_configurations(enabled);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_scheduled ON backup_configurations(is_scheduled);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_next_run ON backup_configurations(next_run_at);

-- Backup jobs table (individual backup operations)
CREATE TABLE IF NOT EXISTS backup_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    configuration_id UUID REFERENCES backup_configurations(id) ON DELETE CASCADE,
    
    -- Job type and target
    job_type VARCHAR(50) NOT NULL CHECK (job_type IN ('app', 'volume', 'database', 'server', 'manual')),
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    database_id UUID REFERENCES databases(id) ON DELETE CASCADE,
    volume_id UUID REFERENCES volumes(id) ON DELETE CASCADE,
    
    -- Job details
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Execution details
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_seconds INTEGER,
    
    -- Progress tracking
    bytes_processed BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT,
    current_phase VARCHAR(100),
    progress_percentage DECIMAL(5,2) NOT NULL DEFAULT 0,
    
    -- Error handling
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    last_retry_at TIMESTAMPTZ,
    
    -- Trigger information
    triggered_by VARCHAR(50) NOT NULL DEFAULT 'manual' CHECK (triggered_by IN ('manual', 'schedule', 'api', 'system')),
    triggered_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- Node execution
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    beacon_task_id VARCHAR(255),
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT backup_job_unique_name UNIQUE (name)
);

-- Indexes for backup_jobs
CREATE INDEX IF NOT EXISTS idx_backup_jobs_configuration ON backup_jobs(configuration_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_server ON backup_jobs(server_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_app ON backup_jobs(app_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_database ON backup_jobs(database_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_volume ON backup_jobs(volume_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_status ON backup_jobs(status);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_created ON backup_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_triggered_by ON backup_jobs(triggered_by);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_node ON backup_jobs(node_id);

-- Backup artifacts table (actual backup files)
CREATE TABLE IF NOT EXISTS backup_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID REFERENCES backup_jobs(id) ON DELETE CASCADE,
    configuration_id UUID REFERENCES backup_configurations(id) ON DELETE CASCADE,
    
    -- Artifact details
    artifact_type VARCHAR(50) NOT NULL CHECK (artifact_type IN ('app', 'volume', 'database', 'server')),
    name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    
    -- Storage information
    storage_provider VARCHAR(50) NOT NULL,
    storage_path VARCHAR(1024) NOT NULL,
    storage_url VARCHAR(2048),
    
    -- File metadata
    file_size BIGINT NOT NULL DEFAULT 0,
    file_hash VARCHAR(128),
    hash_algorithm VARCHAR(50) NOT NULL DEFAULT 'sha256',
    
    -- Content information
    source_server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    source_app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    source_database_id UUID REFERENCES databases(id) ON DELETE CASCADE,
    source_volume_id UUID REFERENCES volumes(id) ON DELETE CASCADE,
    
    -- Database-specific info
    database_engine VARCHAR(50),
    database_name VARCHAR(255),
    
    -- Volume-specific info
    volume_name VARCHAR(255),
    volume_mount_path VARCHAR(1024),
    
    -- App-specific info
    app_name VARCHAR(255),
    app_version VARCHAR(100),
    
    -- Compression and encryption
    is_compressed BOOLEAN NOT NULL DEFAULT FALSE,
    compression_algorithm VARCHAR(50),
    is_encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    encryption_algorithm VARCHAR(50),
    
    -- Status and validation
    status VARCHAR(50) NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'uploading', 'uploaded', 'verifying', 'verified', 'failed', 'corrupted', 'deleted')),
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    verification_attempts INTEGER NOT NULL DEFAULT 0,
    last_verified_at TIMESTAMPTZ,
    
    -- Retention and lifecycle
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    lock_reason VARCHAR(255),
    expires_at TIMESTAMPTZ,
    
    -- Manifest and metadata
    manifest JSONB,
    metadata JSONB,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploaded_at TIMESTAMPTZ,
    
    -- Constraints
    CONSTRAINT backup_artifact_unique_path UNIQUE (storage_provider, storage_path)
);

-- Indexes for backup_artifacts
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_job ON backup_artifacts(job_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_configuration ON backup_artifacts(configuration_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_type ON backup_artifacts(artifact_type);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_status ON backup_artifacts(status);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_source_server ON backup_artifacts(source_server_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_source_database ON backup_artifacts(source_database_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_source_volume ON backup_artifacts(source_volume_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_created ON backup_artifacts(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_locked ON backup_artifacts(is_locked);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_expires ON backup_artifacts(expires_at);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_storage ON backup_artifacts(storage_provider, storage_path);

-- Restore operations table
CREATE TABLE IF NOT EXISTS backup_restores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id UUID REFERENCES backup_artifacts(id) ON DELETE CASCADE,
    job_id UUID REFERENCES backup_jobs(id) ON DELETE SET NULL,
    
    -- Restore target
    restore_type VARCHAR(50) NOT NULL CHECK (restore_type IN ('app', 'volume', 'database', 'server')),
    target_server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    target_app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    target_database_id UUID REFERENCES databases(id) ON DELETE CASCADE,
    target_volume_id UUID REFERENCES volumes(id) ON DELETE CASCADE,
    
    -- Restore details
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Execution details
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'preparing', 'downloading', 'restoring', 'verifying', 'completed', 'failed', 'cancelled', 'rollback')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_seconds INTEGER,
    
    -- Progress tracking
    bytes_processed BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT,
    current_phase VARCHAR(100),
    progress_percentage DECIMAL(5,2) NOT NULL DEFAULT 0,
    
    -- Error handling
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    
    -- Node execution
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    beacon_task_id VARCHAR(255),
    
    -- Restore options
    restore_options JSONB,
    
    -- Verification results
    verification_status VARCHAR(50),
    verification_results JSONB,
    
    -- Rollback information
    can_rollback BOOLEAN NOT NULL DEFAULT FALSE,
    rollback_artifact_id UUID REFERENCES backup_artifacts(id) ON DELETE SET NULL,
    
    -- Trigger information
    triggered_by VARCHAR(50) NOT NULL DEFAULT 'manual' CHECK (triggered_by IN ('manual', 'api', 'system', 'disaster_recovery')),
    triggered_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT backup_restore_unique_name UNIQUE (name),
    CONSTRAINT backup_restore_target_check CHECK (
        (restore_type = 'server' AND target_server_id IS NOT NULL AND target_app_id IS NULL AND target_database_id IS NULL AND target_volume_id IS NULL) OR
        (restore_type = 'app' AND target_server_id IS NULL AND target_app_id IS NOT NULL AND target_database_id IS NULL AND target_volume_id IS NULL) OR
        (restore_type = 'database' AND target_server_id IS NULL AND target_app_id IS NULL AND target_database_id IS NOT NULL AND target_volume_id IS NULL) OR
        (restore_type = 'volume' AND target_server_id IS NULL AND target_app_id IS NULL AND target_database_id IS NULL AND target_volume_id IS NOT NULL)
    )
);

-- Indexes for backup_restores
CREATE INDEX IF NOT EXISTS idx_backup_restores_artifact ON backup_restores(artifact_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_job ON backup_restores(job_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_type ON backup_restores(restore_type);
CREATE INDEX IF NOT EXISTS idx_backup_restores_status ON backup_restores(status);
CREATE INDEX IF NOT EXISTS idx_backup_restores_target_server ON backup_restores(target_server_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_target_database ON backup_restores(target_database_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_created ON backup_restores(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_restores_triggered_by ON backup_restores(triggered_by);

-- Backup verification results table
CREATE TABLE IF NOT EXISTS backup_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id UUID REFERENCES backup_artifacts(id) ON DELETE CASCADE,
    restore_id UUID REFERENCES backup_restores(id) ON DELETE CASCADE,
    
    -- Verification type
    verification_type VARCHAR(50) NOT NULL CHECK (verification_type IN ('checksum', 'integrity', 'restore_test', 'database_connectivity', 'app_functionality')),
    
    -- Verification details
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'passed', 'failed', 'skipped')),
    
    -- Results
    passed_checks INTEGER NOT NULL DEFAULT 0,
    failed_checks INTEGER NOT NULL DEFAULT 0,
    total_checks INTEGER NOT NULL DEFAULT 0,
    
    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_seconds INTEGER,
    
    -- Details
    details JSONB,
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for backup_verifications
CREATE INDEX IF NOT EXISTS idx_backup_verifications_artifact ON backup_verifications(artifact_id);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_restore ON backup_verifications(restore_id);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_type ON backup_verifications(verification_type);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_status ON backup_verifications(status);
CREATE INDEX IF NOT EXISTS idx_backup_verifications_created ON backup_verifications(created_at DESC);

-- Backup retention policies table
CREATE TABLE IF NOT EXISTS backup_retention_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Policy scope
    scope VARCHAR(50) NOT NULL CHECK (scope IN ('global', 'server', 'app', 'database', 'volume')),
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    app_id UUID REFERENCES apps(id) ON DELETE CASCADE,
    database_id UUID REFERENCES databases(id) ON DELETE CASCADE,
    volume_id UUID REFERENCES volumes(id) ON DELETE CASCADE,
    
    -- Retention rules
    max_backups INTEGER,
    retention_days INTEGER,
    retention_weeks INTEGER,
    retention_months INTEGER,
    
    -- Cleanup schedule
    cleanup_schedule VARCHAR(100),
    last_cleanup_at TIMESTAMPTZ,
    next_cleanup_at TIMESTAMPTZ,
    
    -- Priority (lower number = higher priority)
    priority INTEGER NOT NULL DEFAULT 100,
    
    -- Status
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT backup_retention_unique_name UNIQUE (name),
    CONSTRAINT backup_retention_scope_check CHECK (
        (scope = 'global' AND server_id IS NULL AND app_id IS NULL AND database_id IS NULL AND volume_id IS NULL) OR
        (scope = 'server' AND server_id IS NOT NULL AND app_id IS NULL AND database_id IS NULL AND volume_id IS NULL) OR
        (scope = 'app' AND server_id IS NULL AND app_id IS NOT NULL AND database_id IS NULL AND volume_id IS NULL) OR
        (scope = 'database' AND server_id IS NULL AND app_id IS NULL AND database_id IS NOT NULL AND volume_id IS NULL) OR
        (scope = 'volume' AND server_id IS NULL AND app_id IS NULL AND database_id IS NULL AND volume_id IS NOT NULL)
    )
);

-- Indexes for backup_retention_policies
CREATE INDEX IF NOT EXISTS idx_backup_retention_scope ON backup_retention_policies(scope);
CREATE INDEX IF NOT EXISTS idx_backup_retention_server ON backup_retention_policies(server_id);
CREATE INDEX IF NOT EXISTS idx_backup_retention_enabled ON backup_retention_policies(enabled);
CREATE INDEX IF NOT EXISTS idx_backup_retention_priority ON backup_retention_policies(priority);
CREATE INDEX IF NOT EXISTS idx_backup_retention_next_cleanup ON backup_retention_policies(next_cleanup_at);

-- Backup storage providers configuration
CREATE TABLE IF NOT EXISTS backup_storage_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    provider_type VARCHAR(50) NOT NULL CHECK (provider_type IN ('local', 's3', 'minio', 'azure', 'gcs')),
    
    -- Configuration
    config JSONB NOT NULL,
    
    -- Status
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    last_test_at TIMESTAMPTZ,
    last_test_status VARCHAR(50),
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    -- Constraints
    CONSTRAINT backup_storage_unique_name UNIQUE (name)
);

-- Indexes for backup_storage_providers
CREATE INDEX IF NOT EXISTS idx_backup_storage_type ON backup_storage_providers(provider_type);
CREATE INDEX IF NOT EXISTS idx_backup_storage_enabled ON backup_storage_providers(enabled);
CREATE INDEX IF NOT EXISTS idx_backup_storage_default ON backup_storage_providers(is_default);

-- Create triggers for updated_at timestamps
CREATE OR REPLACE FUNCTION update_backup_config_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_config_updated_at_trigger
    BEFORE UPDATE ON backup_configurations
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_config_updated_at();

CREATE OR REPLACE FUNCTION update_backup_job_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_job_updated_at_trigger
    BEFORE UPDATE ON backup_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_job_updated_at();

CREATE OR REPLACE FUNCTION update_backup_artifact_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_artifact_updated_at_trigger
    BEFORE UPDATE ON backup_artifacts
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_artifact_updated_at();

CREATE OR REPLACE FUNCTION update_backup_restore_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_restore_updated_at_trigger
    BEFORE UPDATE ON backup_restores
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_restore_updated_at();

CREATE OR REPLACE FUNCTION update_backup_verification_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_verification_updated_at_trigger
    BEFORE UPDATE ON backup_verifications
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_verification_updated_at();

CREATE OR REPLACE FUNCTION update_backup_retention_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_retention_updated_at_trigger
    BEFORE UPDATE ON backup_retention_policies
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_retention_updated_at();

CREATE OR REPLACE FUNCTION update_backup_storage_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER backup_storage_updated_at_trigger
    BEFORE UPDATE ON backup_storage_providers
    FOR EACH ROW
    EXECUTE FUNCTION update_backup_storage_updated_at();

-- Create a function to calculate next cron run
CREATE OR REPLACE FUNCTION calculate_next_cron_run(cron_expression TEXT)
RETURNS TIMESTAMPTZ AS $$
DECLARE
    next_run TIMESTAMPTZ;
BEGIN
    -- This is a placeholder - in practice, use a proper cron parser
    -- For now, return NULL which will be handled by application logic
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Create views for common queries
CREATE VIEW backup_system_overview AS
SELECT 
    'configurations' as entity_type,
    COUNT(*) as total_count,
    SUM(CASE WHEN enabled = TRUE THEN 1 ELSE 0 END) as active_count
FROM backup_configurations
UNION ALL
SELECT 
    'jobs' as entity_type,
    COUNT(*) as total_count,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as active_count
FROM backup_jobs
UNION ALL
SELECT 
    'artifacts' as entity_type,
    COUNT(*) as total_count,
    SUM(CASE WHEN status = 'verified' THEN 1 ELSE 0 END) as active_count
FROM backup_artifacts
UNION ALL
SELECT 
    'restores' as entity_type,
    COUNT(*) as total_count,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as active_count
FROM backup_restores;

-- Create a view for backup statistics
CREATE VIEW backup_statistics AS
SELECT 
    DATE(created_at) as date,
    artifact_type,
    COUNT(*) as backup_count,
    SUM(file_size) as total_size,
    AVG(file_size) as avg_size
FROM backup_artifacts
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at), artifact_type
ORDER BY date DESC, artifact_type;

-- Add comments to tables for documentation
COMMENT ON TABLE backup_configurations IS 'Stores backup configuration templates for scheduled and on-demand backups';
COMMENT ON TABLE backup_jobs IS 'Tracks individual backup job executions and their status';
COMMENT ON TABLE backup_artifacts IS 'Stores metadata about actual backup files and their storage locations';
COMMENT ON TABLE backup_restores IS 'Tracks restore operations from backup artifacts';
COMMENT ON TABLE backup_verifications IS 'Stores results of backup verification and restore testing';
COMMENT ON TABLE backup_retention_policies IS 'Defines retention rules for automatic backup cleanup';
COMMENT ON TABLE backup_storage_providers IS 'Configuration for different storage backends (local, S3, etc.)';
