-- Canonical multi-tenant workload model. Existing server rows remain valid;
-- modules adopt these tables incrementally through compatibility adapters.

CREATE TABLE IF NOT EXISTS organizations (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization_members (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL DEFAULT 'member',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id),
    CONSTRAINT organization_members_role_check CHECK (role IN ('owner', 'admin', 'member', 'viewer'))
);

CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL,
    slug text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE TABLE IF NOT EXISTS environments (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name text NOT NULL,
    slug text NOT NULL,
    production boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_one_production
    ON environments(project_id) WHERE production;

CREATE TABLE IF NOT EXISTS workloads (
    id uuid PRIMARY KEY,
    environment_id uuid NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    kind text NOT NULL,
    name text NOT NULL,
    desired_generation bigint NOT NULL DEFAULT 1,
    observed_generation bigint NOT NULL DEFAULT 0,
    desired_state text NOT NULL DEFAULT 'stopped',
    observed_state text NOT NULL DEFAULT 'unknown',
    current_revision_id uuid,
    last_observation_at timestamptz,
    last_reconcile_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (environment_id, name),
    CONSTRAINT workloads_kind_check CHECK (kind ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    CONSTRAINT workloads_generation_check CHECK (desired_generation >= observed_generation AND observed_generation >= 0)
);

CREATE TABLE IF NOT EXISTS workload_revisions (
    id uuid PRIMARY KEY,
    workload_id uuid NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    number bigint NOT NULL,
    schema_version integer NOT NULL DEFAULT 1,
    spec jsonb NOT NULL DEFAULT '{}',
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workload_id, number)
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workloads_current_revision_fk') THEN
        ALTER TABLE workloads
            ADD CONSTRAINT workloads_current_revision_fk
            FOREIGN KEY (current_revision_id) REFERENCES workload_revisions(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS workload_instances (
    id uuid PRIMARY KEY,
    workload_id uuid NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    revision_id uuid NOT NULL REFERENCES workload_revisions(id) ON DELETE RESTRICT,
    node_id uuid REFERENCES nodes(id) ON DELETE SET NULL,
    desired_state text NOT NULL DEFAULT 'running',
    observed_state text NOT NULL DEFAULT 'unknown',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workloads_environment ON workloads(environment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_workloads_reconcile ON workloads(desired_generation, observed_generation)
    WHERE desired_generation > observed_generation;
CREATE INDEX IF NOT EXISTS idx_workload_instances_node ON workload_instances(node_id)
    WHERE node_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS workload_observations (
    id uuid PRIMARY KEY,
    workload_id uuid NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    instance_id uuid REFERENCES workload_instances(id) ON DELETE CASCADE,
    generation bigint NOT NULL,
    state text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}',
    observed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workload_observations_workload
    ON workload_observations(workload_id, observed_at DESC);

CREATE TABLE IF NOT EXISTS agent_commands (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    workload_id uuid REFERENCES workloads(id) ON DELETE CASCADE,
    operation_id uuid REFERENCES operations(id) ON DELETE SET NULL,
    command_type text NOT NULL,
    protocol_version integer NOT NULL DEFAULT 1,
    idempotency_key text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'queued',
    acknowledgement jsonb NOT NULL DEFAULT '{}',
    error text NOT NULL DEFAULT '',
    issued_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT agent_commands_status_check CHECK (status IN ('queued', 'delivered', 'acknowledged', 'succeeded', 'failed', 'cancelled')),
    UNIQUE (node_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_agent_commands_delivery
    ON agent_commands(node_id, status, issued_at)
    WHERE status IN ('queued', 'delivered');

-- A deterministic scope lets existing single-tenant installations adopt the
-- canonical model without an interactive migration.
INSERT INTO organizations (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Default Organization', 'default')
ON CONFLICT (id) DO NOTHING;
INSERT INTO projects (id, organization_id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'Default Project', 'default')
ON CONFLICT (id) DO NOTHING;
INSERT INTO environments (id, project_id, name, slug, production)
VALUES ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', 'Production', 'production', true)
ON CONFLICT (id) DO NOTHING;
