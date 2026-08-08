-- Phase 2: env-var groups (named bundles of KEY=VALUE entries scoped to an
-- environment). Group membership to services is declared in the environment
-- manifest (ServiceDef.Groups); the group content itself lives here as JSON
-- so the UI can list/add/remove/rename entries and save without touching
-- environment_variables rows.
CREATE TABLE IF NOT EXISTS env_var_groups (
    id           UUID PRIMARY KEY,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    variables    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_env_var_group UNIQUE (environment_id, name)
);

CREATE INDEX IF NOT EXISTS idx_env_var_groups_env ON env_var_groups (environment_id);
