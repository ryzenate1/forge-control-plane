-- Phase 1: gitlinks substrate (git_sources x env map, webhook event log, OAuth state)

ALTER TABLE git_sources ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE git_sources ADD COLUMN IF NOT EXISTS environment_id UUID REFERENCES environments(id);

CREATE INDEX IF NOT EXISTS git_sources_org_id_idx ON git_sources (org_id);
CREATE INDEX IF NOT EXISTS git_sources_environment_id_idx ON git_sources (environment_id);

-- Event map: every provider webhook (push / PR opened,synchronize,closed) is
-- recorded here so a git source can be bound to an environment and its
-- traffic observed after the fact.
CREATE TABLE IF NOT EXISTS git_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID REFERENCES git_sources(id) ON DELETE SET NULL,
    provider TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL DEFAULT '',
    repository TEXT NOT NULL DEFAULT '',
    ref TEXT NOT NULL DEFAULT '',
    sha TEXT NOT NULL DEFAULT '',
    pr_number INT,
    action TEXT NOT NULL DEFAULT '',
    installation_id TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_webhook_events_source_id_idx ON git_webhook_events (source_id, received_at DESC);
CREATE INDEX IF NOT EXISTS git_webhook_events_received_at_idx ON git_webhook_events (received_at DESC);

-- OAuth connect state: ties a provider redirect back to the authenticated
-- user and a safe post-connect redirect target.
CREATE TABLE IF NOT EXISTS git_oauth_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL UNIQUE,
    redirect_uri TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS git_oauth_states_user_id_idx ON git_oauth_states (user_id);
CREATE INDEX IF NOT EXISTS git_oauth_states_expires_at_idx ON git_oauth_states (expires_at);