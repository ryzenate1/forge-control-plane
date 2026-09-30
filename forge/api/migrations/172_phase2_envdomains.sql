-- Phase 2: per-environment *.domain + TLS provisioning ledger.
--
-- The domainsenv orchestrator scans for environments without a provisioning
-- row (or with a failed one) and ensures a wildcard CNAME
--   *.<envslug>.<base> -> <target>
-- plus an ACME certificate for the wildcard host, recording intent/status
-- here. base comes from the ENV_DOMAIN operator setting; see
-- internal/services/domainsenv/service.go.
CREATE TABLE IF NOT EXISTS env_domain_provisioning (
    env_id        UUID PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
    domain        TEXT NOT NULL DEFAULT '',
    wildcard_host TEXT NOT NULL DEFAULT '',
    target        TEXT NOT NULL DEFAULT '',
    dns_status    TEXT NOT NULL DEFAULT 'pending',
    tls_status    TEXT NOT NULL DEFAULT 'pending',
    -- certificates.id is VARCHAR(36), so the reference must match that type.
    cert_id       VARCHAR(36) REFERENCES certificates(id) ON DELETE SET NULL,
    last_error    TEXT NOT NULL DEFAULT '',
    attempted_at  TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_env_domain_provisioning_status
    ON env_domain_provisioning (dns_status, tls_status);
