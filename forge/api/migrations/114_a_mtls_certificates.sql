CREATE TYPE cert_type AS ENUM ('ca', 'server', 'client');

CREATE TABLE IF NOT EXISTS mtls_certificates (
    id VARCHAR(36) PRIMARY KEY,
    cert_type cert_type NOT NULL DEFAULT 'client',
    common_name VARCHAR(255) NOT NULL DEFAULT '',
    organization VARCHAR(255) NOT NULL DEFAULT '',
    certificate_pem TEXT NOT NULL DEFAULT '',
    private_key_encrypted TEXT NOT NULL DEFAULT '',
    serial_number VARCHAR(64) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    node_id VARCHAR(36) REFERENCES nodes(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mtls_certificates_cert_type ON mtls_certificates(cert_type);
CREATE INDEX IF NOT EXISTS idx_mtls_certificates_node_id ON mtls_certificates(node_id);
CREATE INDEX IF NOT EXISTS idx_mtls_certificates_expires_at ON mtls_certificates(expires_at);
CREATE INDEX IF NOT EXISTS idx_mtls_certificates_revoked_at ON mtls_certificates(revoked_at) WHERE revoked_at IS NULL;
