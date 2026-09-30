-- HashiCorp Vault provider integration (inspired by Dokploy's vault-provider).
--
-- A vault_connections row registers an external secret store (a Vault instance)
-- the control plane can resolve environment-variable references against at
-- deploy time, instead of duplicating the secret into Forge's own encrypted
-- store. Auth is either a raw token or an AppRole (role_id + secret_id).
--
-- The token and the AppRole secret_id are credentials in their own right, so
-- they are stored only in `*_encrypted` envelope columns (AES-GCM via the
-- panel keyring, AAD namespaced to this table + row id + field, matching the
-- environment_variables convention). The reversible legacy plaintext columns
-- are kept NULL/'' so a rolled-back binary can still read them; the encrypted
-- column is authoritative. role_id is a non-secret identifier (like a username)
-- and is stored in the clear.
--
-- Connections are org-scoped when org_id is set; a NULL org_id is a global
-- (panel-wide) connection. The partial unique indexes below enforce one
-- connection per (base_url, mount_path, namespace) for globals and, separately,
-- per org for org-scoped rows, so a global and an org row never collide.

CREATE TABLE IF NOT EXISTS vault_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    mount_path TEXT NOT NULL DEFAULT 'secret',
    namespace TEXT,
    engine_version INT NOT NULL DEFAULT 2 CHECK (engine_version IN (1, 2)),
    auth_method VARCHAR(16) NOT NULL DEFAULT 'token' CHECK (auth_method IN ('token', 'approle')),
    -- Credential envelopes (authoritative) + reversible legacy plaintext.
    token_encrypted TEXT NOT NULL DEFAULT '',
    token_plaintext TEXT NOT NULL DEFAULT '',
    role_id TEXT NOT NULL DEFAULT '',
    secret_id_encrypted TEXT NOT NULL DEFAULT '',
    secret_id_plaintext TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Global connections: unique per endpoint triple where org_id is NULL.
CREATE UNIQUE INDEX IF NOT EXISTS vault_connections_global_uq
    ON vault_connections (lower(base_url), mount_path, COALESCE(namespace, ''))
    WHERE org_id IS NULL;

-- Org-scoped connections: unique per (org, endpoint triple) otherwise.
CREATE UNIQUE INDEX IF NOT EXISTS vault_connections_org_uq
    ON vault_connections (org_id, lower(base_url), mount_path, COALESCE(namespace, ''))
    WHERE org_id IS NOT NULL;

-- Human-friendly name is unique within its scope so a reference's connection id
-- stays unambiguous and the admin list reads cleanly.
CREATE UNIQUE INDEX IF NOT EXISTS vault_connections_org_name_uq
    ON vault_connections (COALESCE(org_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

CREATE INDEX IF NOT EXISTS idx_vault_connections_org ON vault_connections (org_id);
CREATE INDEX IF NOT EXISTS idx_vault_connections_enabled ON vault_connections (enabled);
