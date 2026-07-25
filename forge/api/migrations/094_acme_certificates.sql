CREATE TABLE IF NOT EXISTS certificates (
    id VARCHAR(36) PRIMARY KEY,
    domains TEXT[] NOT NULL DEFAULT '{}',
    issuer VARCHAR(255) NOT NULL DEFAULT '',
    certificate TEXT NOT NULL DEFAULT '',
    private_key_encrypted TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    auto_renew BOOLEAN NOT NULL DEFAULT true,
    provider VARCHAR(100) NOT NULL DEFAULT '',
    challenge_type VARCHAR(20) NOT NULL DEFAULT 'http-01',
    wildcard BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS certificate_attempts (
    id VARCHAR(36) PRIMARY KEY,
    certificate_id VARCHAR(36) NOT NULL REFERENCES certificates(id) ON DELETE CASCADE,
    attempt_type VARCHAR(50) NOT NULL DEFAULT 'issue',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    domains TEXT[] NOT NULL DEFAULT '{}',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_certificates_expires_at ON certificates(expires_at);
CREATE INDEX IF NOT EXISTS idx_certificates_auto_renew ON certificates(auto_renew) WHERE auto_renew = true;
CREATE INDEX IF NOT EXISTS idx_certificate_attempts_certificate_id ON certificate_attempts(certificate_id);
CREATE INDEX IF NOT EXISTS idx_certificate_attempts_status ON certificate_attempts(status);
