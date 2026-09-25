-- Per-application domain redirects and forwards.
--
-- Inspired by Dokploy's `redirects` router: one row per redirect an operator
-- configures for an application, rendered into gateway configuration (Traefik
-- redirectRegex / redirectScheme middlewares, Caddy redirection routes) by
-- internal/services/redirects.GenerateGatewayConfig.
--
-- Why a new table instead of reusing `redirect_rules`: that one is keyed to a
-- proxy *domain* and managed by an administrator, whereas these rules belong to
-- an application and are edited by the people who own that application. The two
-- differ in owner, in lifetime (these cascade with the application) and in who
-- may write them, so sharing a table would mean sharing every one of those
-- compromises.
--
-- `target_path` and `preset_type` are deliberately nullable and the nulls mean
-- different things than empty strings:
--   * target_path IS NULL  -> "preserve the incoming request path", which is
--     what a host canonicalisation wants; an empty target_path means "send
--     everything to the target root". Collapsing the two would silently flatten
--     every deep link a www->apex rule was supposed to carry over.
--   * preset_type IS NULL  -> the row predates presets or was written directly
--     through the API; the value is provenance for the UI and the idempotency
--     check in ApplyPresets, never an input to matching.
--
-- Postgres is the production dialect; the statements below are restricted to
-- syntax the SQLite dev fallback can execute after the migration runner's type
-- substitutions (UUID -> TEXT, TIMESTAMPTZ -> TIMESTAMP, gen_random_uuid() ->
-- randomblob expression). No regex CHECKs, no partial indexes, no COMMENT ON --
-- `substr(x, 1, 1) = '/'` is used instead of a LIKE pattern so both engines
-- evaluate the same predicate.
--
-- Numbering: the requested 228_ prefix is already taken by the bare
-- 228_upgrade_plans.sql, and validateNoDuplicatePrefixes rejects a second bare
-- 228, so this ships as a letter-suffixed sibling of 228 the way 221_a and
-- 225_a do elsewhere in this directory.

CREATE TABLE IF NOT EXISTS domain_redirects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    source_domain VARCHAR(253) NOT NULL,
    target_domain VARCHAR(253) NOT NULL,
    source_path TEXT NOT NULL DEFAULT '/',
    target_path TEXT,
    status_code INTEGER NOT NULL DEFAULT 301 CHECK (status_code IN (301, 302, 307, 308)),
    preset_type VARCHAR(32) CHECK (preset_type IS NULL OR preset_type IN ('http-to-https', 'www-apex', 'apex-www', 'custom')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT domain_redirects_source_path_absolute CHECK (substr(source_path, 1, 1) = '/'),
    CONSTRAINT domain_redirects_target_path_absolute CHECK (target_path IS NULL OR substr(target_path, 1, 1) = '/'),
    -- Two rules with the same (application, source host, source path, target
    -- host) cannot both be honoured: the gateway evaluates them in one order,
    -- so the second becomes unreachable while still looking active in the UI.
    -- The status code and target path are excluded from the identity on purpose
    -- — "the same pair of endpoints described twice" is exactly the mistake
    -- worth refusing.
    CONSTRAINT domain_redirects_rule_identity_unique UNIQUE (application_id, source_domain, source_path, target_domain),
    -- A rule that changes nothing is an infinite redirect: the gateway answers
    -- 3xx with the URL it just served. The one exception is an HTTP->HTTPS
    -- upgrade, whose source and target host are identical by construction
    -- because only the scheme differs (the scheme is not a column; it is what
    -- the preset label means).
    CONSTRAINT domain_redirects_changes_something CHECK (
        source_domain <> target_domain
        OR (preset_type IS NOT NULL AND preset_type = 'http-to-https')
    )
);

-- Every read this feature performs is scoped to one application: the panel
-- list, the validation pass that loads sibling rules to detect a reverse
-- redirect, and the gateway renderer.
CREATE INDEX IF NOT EXISTS idx_domain_redirects_application_id ON domain_redirects(application_id);
-- The gateway renderer keeps only enabled rules, so it is the hot filter as
-- soon as an application has more than a handful.
CREATE INDEX IF NOT EXISTS idx_domain_redirects_app_enabled ON domain_redirects(application_id, enabled);
-- Guarded by the same uniqueness the service checks before inserting; the index
-- on (source_domain) is what a "which rules point at this host" lookup needs
-- when a domain is detached from an application.
CREATE INDEX IF NOT EXISTS idx_domain_redirects_source_domain ON domain_redirects(source_domain);
