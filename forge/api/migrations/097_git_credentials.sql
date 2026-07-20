CREATE TABLE IF NOT EXISTS git_credentials (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    credential_type TEXT NOT NULL CHECK (credential_type IN ('ssh_key', 'https_password', 'https_token')),
    credential_encrypted TEXT NOT NULL DEFAULT '',
    credential_plaintext TEXT NOT NULL DEFAULT '',
    public_key TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_credentials_user_id_idx ON git_credentials (user_id);

CREATE TABLE IF NOT EXISTS git_provider_tokens (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (provider IN ('github', 'gitlab', 'bitbucket', 'gitea')),
    provider_name TEXT NOT NULL DEFAULT '',
    access_token_encrypted TEXT NOT NULL DEFAULT '',
    access_token_plaintext TEXT NOT NULL DEFAULT '',
    refresh_token_encrypted TEXT NOT NULL DEFAULT '',
    refresh_token_plaintext TEXT NOT NULL DEFAULT '',
    token_type TEXT NOT NULL DEFAULT 'bearer',
    expires_at TIMESTAMPTZ,
    scope TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_provider_tokens_user_id_idx ON git_provider_tokens (user_id);

CREATE TABLE IF NOT EXISTS git_sources (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id TEXT NOT NULL,
    credential_id TEXT,
    provider_token_id TEXT,
    provider TEXT NOT NULL DEFAULT '' CHECK (provider IN ('', 'github', 'gitlab', 'bitbucket', 'gitea', 'custom')),
    repository_url TEXT NOT NULL,
    repository_name TEXT NOT NULL DEFAULT '',
    repository_owner TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT 'main',
    auto_deploy BOOLEAN NOT NULL DEFAULT false,
    webhook_secret_encrypted TEXT NOT NULL DEFAULT '',
    webhook_secret_plaintext TEXT NOT NULL DEFAULT '',
    webhook_id TEXT NOT NULL DEFAULT '',
    webhook_url TEXT NOT NULL DEFAULT '',
    last_commit_sha TEXT NOT NULL DEFAULT '',
    last_commit_message TEXT NOT NULL DEFAULT '',
    last_commit_author TEXT NOT NULL DEFAULT '',
    last_deployed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS git_sources_user_id_idx ON git_sources (user_id);
CREATE INDEX IF NOT EXISTS git_sources_credential_id_idx ON git_sources (credential_id);
CREATE INDEX IF NOT EXISTS git_sources_provider_token_id_idx ON git_sources (provider_token_id);
