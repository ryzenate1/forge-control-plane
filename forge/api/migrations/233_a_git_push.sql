-- Dokku-style "git push to deploy" applications.
--
-- A git_push_apps row owns a bare repository on one Beacon node. The user
-- pushes to it over SSH (RemoteURL below), the repository's post-receive hook
-- calls back into POST /api/v1/git-push/receive/:slug carrying the raw
-- "<old> <new> <ref>" lines the hook read from stdin, authenticated by an
-- HMAC-SHA256 of the body keyed with shared_secret. Every callback is recorded
-- in git_push_events; when auto_deploy is on the push is also handed to the
-- existing build/deploy pipeline.
--
-- Builder mirrors the Dokku builder choice (herokuish / dockerfile / nixpacks /
-- null); status tracks provisioning because creating the bare repository on the
-- node is a two-step handshake with Beacon and may not complete inline.

CREATE TABLE IF NOT EXISTS git_push_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    -- Optional workload the deploys land on. Text-typed to stay agnostic of
    -- the servers identifier spelling (see 143_compose_stack_text_identifiers).
    server_id TEXT NULL,
    builder TEXT NOT NULL DEFAULT 'dockerfile' CHECK (builder IN ('herokuish', 'dockerfile', 'nixpacks', 'null')),
    branch TEXT NOT NULL DEFAULT 'main',
    repo_path TEXT NOT NULL DEFAULT '',
    shared_secret TEXT NOT NULL DEFAULT '',
    deployed_sha TEXT NULL,
    last_deploy_at TIMESTAMPTZ NULL,
    -- provisioning | ready | failed | archived
    status TEXT NOT NULL DEFAULT 'provisioning',
    auto_deploy BOOLEAN NOT NULL DEFAULT true,
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_git_push_apps_node ON git_push_apps(node_id);
CREATE INDEX IF NOT EXISTS idx_git_push_apps_status ON git_push_apps(status);

CREATE TABLE IF NOT EXISTS git_push_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL REFERENCES git_push_apps(id) ON DELETE CASCADE,
    ref TEXT NOT NULL DEFAULT '',
    before_sha TEXT NOT NULL DEFAULT '',
    after_sha TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    -- received | queued | deploying | deployed | failed | skipped
    status TEXT NOT NULL DEFAULT 'received',
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_git_push_events_app ON git_push_events(app_id);
CREATE INDEX IF NOT EXISTS idx_git_push_events_created ON git_push_events(created_at DESC);
