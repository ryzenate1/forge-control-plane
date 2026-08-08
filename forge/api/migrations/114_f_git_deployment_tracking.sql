-- Migration: Git Deployment Tracking
-- Up: Create tables for tracking Git deployments

-- Git deployments table for tracking deployment status and history
CREATE TABLE IF NOT EXISTS git_deployments (
    id VARCHAR(36) PRIMARY KEY,
    git_source_id VARCHAR(36) NOT NULL,
    commit_sha VARCHAR(64) NOT NULL,
    branch VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    status_message TEXT,
    image_tag VARCHAR(512),
    build_log TEXT,
    deploy_log TEXT,
    error TEXT,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_git_deployments_source ON git_deployments(git_source_id);
CREATE INDEX IF NOT EXISTS idx_git_deployments_commit ON git_deployments(commit_sha);
CREATE INDEX IF NOT EXISTS idx_git_deployments_status ON git_deployments(status);
CREATE INDEX IF NOT EXISTS idx_git_deployments_created ON git_deployments(created_at);

-- Add deployment tracking columns to git_sources table if they don't exist
ALTER TABLE git_sources 
ADD COLUMN IF NOT EXISTS last_deployment_id VARCHAR(36),
ADD COLUMN IF NOT EXISTS last_deployment_status VARCHAR(20),
ADD COLUMN IF NOT EXISTS last_deployment_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS deployment_count INTEGER DEFAULT 0;

-- Create index for deployment tracking
CREATE INDEX IF NOT EXISTS idx_git_sources_deployment ON git_sources(last_deployment_id, last_deployment_status);

-- Down: Drop tables and columns (for rollback)
-- DROP TABLE IF EXISTS git_deployments;
-- ALTER TABLE git_sources DROP COLUMN IF EXISTS last_deployment_id;
-- ALTER TABLE git_sources DROP COLUMN IF EXISTS last_deployment_status;
-- ALTER TABLE git_sources DROP COLUMN IF EXISTS last_deployment_at;
-- ALTER TABLE git_sources DROP COLUMN IF EXISTS deployment_count;