CREATE TABLE IF NOT EXISTS acme_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    private_key TEXT,
    ca_url VARCHAR(500) NOT NULL DEFAULT 'https://acme-v02.api.letsencrypt.org/directory',
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dns_provider_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(100) NOT NULL,
    credentials JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_acme_accounts_is_default ON acme_accounts(is_default) WHERE is_default = TRUE;
CREATE INDEX IF NOT EXISTS idx_dns_provider_accounts_provider ON dns_provider_accounts(provider);
