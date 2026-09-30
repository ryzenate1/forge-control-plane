CREATE TABLE IF NOT EXISTS proxy_domains (
    id UUID PRIMARY KEY,
    hostname TEXT NOT NULL,
    service_id TEXT,
    service_type TEXT NOT NULL DEFAULT 'server',
    https BOOLEAN NOT NULL DEFAULT FALSE,
    port INTEGER NOT NULL DEFAULT 8080,
    cert_type TEXT NOT NULL DEFAULT 'none',
    cert_data TEXT DEFAULT '',
    cert_key TEXT DEFAULT '',
    auto_renew BOOLEAN NOT NULL DEFAULT FALSE,
    path TEXT NOT NULL DEFAULT '/',
    strip_path BOOLEAN NOT NULL DEFAULT FALSE,
    forward_auth_url TEXT DEFAULT '',
    forward_auth_headers JSONB DEFAULT '[]'::jsonb,
    websocket BOOLEAN NOT NULL DEFAULT FALSE,
    rate_limit INTEGER DEFAULT 0,
    rate_limit_burst INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS proxy_certificates (
    id UUID PRIMARY KEY,
    domain_id UUID REFERENCES proxy_domains(id) ON DELETE CASCADE,
    private_key TEXT DEFAULT '',
    certificate TEXT DEFAULT '',
    issuer TEXT DEFAULT '',
    auto_renew BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redirect_rules (
    id UUID PRIMARY KEY,
    domain_id UUID REFERENCES proxy_domains(id) ON DELETE CASCADE,
    source_path TEXT NOT NULL DEFAULT '/',
    target_url TEXT NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 302,
    regex BOOLEAN NOT NULL DEFAULT FALSE,
    preserve_path BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS security_headers (
    id UUID PRIMARY KEY,
    domain_id UUID REFERENCES proxy_domains(id) ON DELETE CASCADE,
    hsts_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    hsts_max_age INTEGER NOT NULL DEFAULT 63072000,
    hsts_include_subdomains BOOLEAN NOT NULL DEFAULT TRUE,
    hsts_preload BOOLEAN NOT NULL DEFAULT FALSE,
    x_frame_options TEXT NOT NULL DEFAULT 'DENY',
    x_content_type_options TEXT NOT NULL DEFAULT 'nosniff',
    referrer_policy TEXT NOT NULL DEFAULT 'strict-origin-when-cross-origin',
    csp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    csp_policy TEXT DEFAULT '',
    permissions_policy TEXT DEFAULT '',
    custom_headers JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_proxy_domains_hostname ON proxy_domains(hostname);
CREATE INDEX IF NOT EXISTS idx_proxy_domains_service ON proxy_domains(service_id, service_type);
CREATE INDEX IF NOT EXISTS idx_redirect_rules_domain ON redirect_rules(domain_id);
CREATE INDEX IF NOT EXISTS idx_security_headers_domain ON security_headers(domain_id);
CREATE INDEX IF NOT EXISTS idx_proxy_certificates_domain ON proxy_certificates(domain_id);
