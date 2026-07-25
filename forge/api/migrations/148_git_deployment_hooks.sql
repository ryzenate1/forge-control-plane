CREATE TABLE IF NOT EXISTS git_deployment_hooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_source_id UUID NOT NULL REFERENCES git_sources(id) ON DELETE CASCADE,
    secret TEXT NOT NULL,
    events TEXT[] NOT NULL DEFAULT ARRAY['push']::TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_deployment_hooks_source_idx
    ON git_deployment_hooks (git_source_id);
