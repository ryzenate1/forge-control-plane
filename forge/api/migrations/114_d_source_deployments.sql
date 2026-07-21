CREATE TABLE IF NOT EXISTS git_providers (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('github', 'gitlab', 'bitbucket', 'gitea', 'generic')),
    access_token_encrypted TEXT DEFAULT '',
    access_token_plaintext TEXT DEFAULT '',
    refresh_token_encrypted TEXT DEFAULT '',
    refresh_token_plaintext TEXT DEFAULT '',
    token_type TEXT DEFAULT 'bearer',
    expires_at TIMESTAMPTZ,
    scope TEXT DEFAULT '',
    base_url TEXT DEFAULT '',
    username TEXT DEFAULT '',
    avatar_url TEXT DEFAULT '',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS source_deployments (
    id UUID PRIMARY KEY,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    git_provider_id UUID REFERENCES git_providers(id) ON DELETE SET NULL,
    repository TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    build_type TEXT NOT NULL DEFAULT 'dockerfile' CHECK (build_type IN ('dockerfile', 'nixpacks', 'heroku', 'paketo', 'static')),
    build_context TEXT DEFAULT '.',
    dockerfile_path TEXT DEFAULT 'Dockerfile',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'queued', 'cloning', 'building', 'pushing', 'deploying', 'healthy', 'unhealthy', 'completed', 'failed', 'canceled')),
    commit_hash TEXT DEFAULT '',
    commit_message TEXT DEFAULT '',
    commit_author TEXT DEFAULT '',
    image_tag TEXT DEFAULT '',
    registry TEXT DEFAULT '',
    registry_credential_id TEXT DEFAULT '',
    auto_deploy BOOLEAN DEFAULT false,
    webhook_id TEXT DEFAULT '',
    webhook_url TEXT DEFAULT '',
    health_check_path TEXT DEFAULT '/',
    health_check_port INT DEFAULT 80,
    rollback_on_health_failure BOOLEAN DEFAULT false,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS source_build_logs (
    id UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES source_deployments(id) ON DELETE CASCADE,
    stage TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_source_deployments_server_id ON source_deployments(server_id);
CREATE INDEX IF NOT EXISTS idx_source_deployments_git_provider_id ON source_deployments(git_provider_id);
CREATE INDEX IF NOT EXISTS idx_source_deployments_status ON source_deployments(status);
CREATE INDEX IF NOT EXISTS idx_source_build_logs_deployment_id ON source_build_logs(deployment_id);
CREATE INDEX IF NOT EXISTS idx_source_build_logs_created_at ON source_build_logs(created_at);
