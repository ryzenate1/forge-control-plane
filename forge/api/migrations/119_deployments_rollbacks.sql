CREATE TABLE IF NOT EXISTS deployment_history (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL,
    service_id UUID,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'error', 'cancelled')),
    log_path TEXT DEFAULT '',
    commit_hash TEXT DEFAULT '',
    commit_message TEXT DEFAULT '',
    error_message TEXT DEFAULT '',
    rollback_id UUID,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rollbacks (
    id UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployment_history(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS preview_deployments (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL,
    service_id UUID,
    pr_number INTEGER NOT NULL DEFAULT 0,
    pr_title TEXT DEFAULT '',
    pr_url TEXT DEFAULT '',
    branch TEXT DEFAULT '',
    repo_owner TEXT DEFAULT '',
    repo_name TEXT DEFAULT '',
    commit_sha TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'deploying' CHECK (status IN ('deploying', 'running', 'stopped', 'failed', 'cleaned_up')),
    preview_url TEXT DEFAULT '',
    deployment_url TEXT DEFAULT '',
    source TEXT NOT NULL DEFAULT 'github' CHECK (source IN ('github', 'gitlab')),
    unique_suffix TEXT DEFAULT '',
    is_isolated BOOLEAN DEFAULT true,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    cleaned_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deployment_history_server_id ON deployment_history(server_id);
CREATE INDEX IF NOT EXISTS idx_deployment_history_status ON deployment_history(status);
CREATE INDEX IF NOT EXISTS idx_deployment_history_created_at ON deployment_history(created_at);
CREATE INDEX IF NOT EXISTS idx_rollbacks_deployment_id ON rollbacks(deployment_id);
CREATE INDEX IF NOT EXISTS idx_preview_deployments_server_id ON preview_deployments(server_id);
CREATE INDEX IF NOT EXISTS idx_preview_deployments_status ON preview_deployments(status);
