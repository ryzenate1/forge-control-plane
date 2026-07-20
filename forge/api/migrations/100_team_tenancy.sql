-- Organizations (teams): Coolify-style team-based tenancy
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    owner_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS organizations_slug_idx ON organizations (slug);
CREATE INDEX IF NOT EXISTS organizations_owner_idx ON organizations (owner_id);

-- Projects: Dokploy-style project grouping under an org
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

CREATE INDEX IF NOT EXISTS projects_org_id_idx ON projects (org_id);

-- Environments: per-project deployment environments (dev/staging/prod)
CREATE TABLE IF NOT EXISTS environments (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT '#6366f1',
    protected BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS environments_project_id_idx ON environments (project_id);

-- Team members: organization-level membership with roles
CREATE TABLE IF NOT EXISTS team_members (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS team_members_org_id_idx ON team_members (org_id);
CREATE INDEX IF NOT EXISTS team_members_user_id_idx ON team_members (user_id);

-- Add org_id to resources for scoping
ALTER TABLE servers ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE backups ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE domains ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE git_sources ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE compose_projects ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);

CREATE INDEX IF NOT EXISTS servers_org_id_idx ON servers (org_id);
CREATE INDEX IF NOT EXISTS deployments_org_id_idx ON deployments (org_id);
CREATE INDEX IF NOT EXISTS backups_org_id_idx ON backups (org_id);
CREATE INDEX IF NOT EXISTS domains_org_id_idx ON domains (org_id);
CREATE INDEX IF NOT EXISTS git_sources_org_id_idx ON git_sources (org_id);
CREATE INDEX IF NOT EXISTS compose_projects_org_id_idx ON compose_projects (org_id);

-- Add project_id and env_id to servers for hierarchy scoping
ALTER TABLE servers ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id);
ALTER TABLE servers ADD COLUMN IF NOT EXISTS environment_id UUID REFERENCES environments(id);

CREATE INDEX IF NOT EXISTS servers_project_id_idx ON servers (project_id);
CREATE INDEX IF NOT EXISTS servers_env_id_idx ON servers (environment_id);
