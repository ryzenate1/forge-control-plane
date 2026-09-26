-- Portainer-style custom stack templates.
--
-- A compose_templates row is a reusable, parameterized docker-compose document:
-- the raw compose_yaml holds ${PARAM_KEY} placeholders and `parameters` records
-- how each placeholder is rendered in the UI (text/number/select/password/
-- env-file). Instantiating a template substitutes the placeholders and deploys
-- the result as a compose stack; every instantiation is recorded in
-- compose_template_instances so operators can trace stacks back to their
-- template and the exact values used.

CREATE TABLE IF NOT EXISTS compose_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    compose_yaml TEXT NOT NULL DEFAULT '',
    -- Ordered list of parameter descriptors:
    -- [{ "key": "...", "label": "...", "type": "text|number|select|password|env-file",
    --    "default": "...", "required": true, "options": ["..."], "secret": false }]
    parameters JSONB NOT NULL DEFAULT '[]'::jsonb,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_compose_templates_category ON compose_templates(category);
CREATE INDEX IF NOT EXISTS idx_compose_templates_visibility ON compose_templates(visibility);
CREATE INDEX IF NOT EXISTS idx_compose_templates_created_by ON compose_templates(created_by);

CREATE TABLE IF NOT EXISTS compose_template_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES compose_templates(id) ON DELETE CASCADE,
    -- The compose stack produced by this instantiation. Stored as text to stay
    -- agnostic of the compose_stacks identifier type (see 143_compose_stack_text_identifiers).
    stack_id TEXT NOT NULL DEFAULT '',
    -- Map of parameter key -> chosen value at instantiate time.
    values_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_compose_template_instances_template ON compose_template_instances(template_id);
CREATE INDEX IF NOT EXISTS idx_compose_template_instances_stack ON compose_template_instances(stack_id);
CREATE INDEX IF NOT EXISTS idx_compose_template_instances_created ON compose_template_instances(created_at DESC);
