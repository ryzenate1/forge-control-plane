-- Applications: unified workload identity for app-hosting.
-- Acts as an umbrella over servers (game) and compose projects (app).
-- NOTE: Migration ID 101 — coordinate with other agents.

CREATE TABLE IF NOT EXISTS applications (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    environment_id UUID REFERENCES environments(id) ON DELETE SET NULL,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    source_type TEXT NOT NULL DEFAULT 'DOCKER_IMAGE' CHECK (source_type IN ('GIT', 'DOCKER_IMAGE', 'COMPOSE')),
    source_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    desired_state TEXT NOT NULL DEFAULT 'running' CHECK (desired_state IN ('running', 'stopped', 'removed')),
    observed_status TEXT NOT NULL DEFAULT 'idle' CHECK (observed_status IN ('idle', 'running', 'done', 'error', 'deploying')),
    current_deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_applications_org_id ON applications (org_id);
CREATE INDEX IF NOT EXISTS idx_applications_project_id ON applications (project_id);
CREATE INDEX IF NOT EXISTS idx_applications_environment_id ON applications (environment_id);
CREATE INDEX IF NOT EXISTS idx_applications_server_id ON applications (server_id);
CREATE INDEX IF NOT EXISTS idx_applications_source_type ON applications (source_type);
CREATE INDEX IF NOT EXISTS idx_applications_org_created ON applications (org_id, created_at DESC);

-- App Services: one-to-many service representation under an application.
-- Supports service dependencies and per-service state tracking.

CREATE TABLE IF NOT EXISTS app_services (
    id UUID PRIMARY KEY,
    app_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    image TEXT NOT NULL DEFAULT '',
    compose_service TEXT NOT NULL DEFAULT '',
    replicas INTEGER NOT NULL DEFAULT 1,
    ports JSONB NOT NULL DEFAULT '[]'::jsonb,
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    depends_on JSONB NOT NULL DEFAULT '[]'::jsonb,
    desired_state TEXT NOT NULL DEFAULT 'running' CHECK (desired_state IN ('running', 'stopped')),
    observed_status TEXT NOT NULL DEFAULT 'idle' CHECK (observed_status IN ('idle', 'running', 'done', 'error', 'deploying')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_services_app_id ON app_services (app_id);
