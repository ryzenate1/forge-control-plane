-- Environment Variables: hierachical env vars at org/project/env/service levels
CREATE TABLE IF NOT EXISTS environment_variables (
    id UUID PRIMARY KEY,
    org_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    environment_id UUID REFERENCES environments(id) ON DELETE CASCADE,
    service_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    scope TEXT NOT NULL DEFAULT 'environment',
    key TEXT NOT NULL,
    value_encrypted TEXT NOT NULL DEFAULT '',
    is_sensitive BOOLEAN NOT NULL DEFAULT false,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_scope CHECK (
        (scope = 'organization' AND org_id IS NOT NULL AND project_id IS NULL AND environment_id IS NULL AND service_id IS NULL) OR
        (scope = 'project' AND org_id IS NULL AND project_id IS NOT NULL AND environment_id IS NULL AND service_id IS NULL) OR
        (scope = 'environment' AND org_id IS NULL AND project_id IS NULL AND environment_id IS NOT NULL AND service_id IS NULL) OR
        (scope = 'service' AND org_id IS NULL AND project_id IS NULL AND environment_id IS NULL AND service_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS env_vars_org_idx ON environment_variables (org_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS env_vars_project_idx ON environment_variables (project_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS env_vars_env_idx ON environment_variables (environment_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS env_vars_service_idx ON environment_variables (service_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS env_vars_org_key_unique ON environment_variables (org_id, key) WHERE scope='organization' AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS env_vars_project_key_unique ON environment_variables (project_id, key) WHERE scope='project' AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS env_vars_env_key_unique ON environment_variables (environment_id, key) WHERE scope='environment' AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS env_vars_service_key_unique ON environment_variables (service_id, key) WHERE scope='service' AND deleted_at IS NULL;

-- Environment variable revision history
CREATE TABLE IF NOT EXISTS environment_variable_revisions (
    id UUID PRIMARY KEY,
    variable_id UUID NOT NULL REFERENCES environment_variables(id) ON DELETE CASCADE,
    version INT NOT NULL,
    value_encrypted TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS env_var_revisions_var_idx ON environment_variable_revisions (variable_id);

-- Organization invitations
CREATE TABLE IF NOT EXISTS org_invitations (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    token TEXT NOT NULL UNIQUE,
    invited_by UUID REFERENCES users(id),
    accepted_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS org_invitations_org_idx ON org_invitations (org_id);
CREATE INDEX IF NOT EXISTS org_invitations_token_idx ON org_invitations (token);
CREATE INDEX IF NOT EXISTS org_invitations_email_idx ON org_invitations (email);

-- Granular permissions for organization members
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS permissions JSONB NOT NULL DEFAULT '{}';

-- Add scope columns to servers for project/environment resolution
ALTER TABLE servers ADD COLUMN IF NOT EXISTS environment_id UUID REFERENCES environments(id);
ALTER TABLE servers ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id);

CREATE INDEX IF NOT EXISTS servers_env_id_idx ON servers (environment_id);
CREATE INDEX IF NOT EXISTS servers_project_id_idx ON servers (project_id);
