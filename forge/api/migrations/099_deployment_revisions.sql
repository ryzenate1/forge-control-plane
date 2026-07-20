ALTER TABLE deployments ADD COLUMN IF NOT EXISTS current_revision_id UUID;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollout_strategy TEXT NOT NULL DEFAULT 'recreate';

CREATE TABLE IF NOT EXISTS deployment_revisions (
    id UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    revision_number INTEGER NOT NULL,
    image_ref TEXT NOT NULL DEFAULT '',
    compose_manifest_ref TEXT NOT NULL DEFAULT '',
    git_commit_sha TEXT NOT NULL DEFAULT '',
    config_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    deployed_at TIMESTAMPTZ,
    description TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_revisions_deployment ON deployment_revisions(deployment_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_revisions_rev_number ON deployment_revisions(deployment_id, revision_number);
