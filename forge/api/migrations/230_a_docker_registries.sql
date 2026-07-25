-- Docker registry credentials for pulling/pushing private images.
-- Mirrors the git_credentials pattern: the secret material is stored encrypted
-- (credential_encrypted) via the panel keyring and never returned masked-free
-- except through the explicit unmasked getter used at deploy time.
CREATE TABLE IF NOT EXISTS docker_registries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    server_address TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    credential_encrypted TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    is_global BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS docker_registries_user_id_idx ON docker_registries (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS docker_registries_name_idx ON docker_registries (name);
