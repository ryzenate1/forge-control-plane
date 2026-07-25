-- Phase 7 (Commerce): billing webhook receiver + settings.
-- billing_webhook_events records every verified call for audit and
-- idempotency (event_id unique per provider). billing_settings holds the
-- shared HMAC secret used to sign webhooks; rotate it from the admin API.
CREATE TABLE IF NOT EXISTS billing_webhook_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider    TEXT NOT NULL DEFAULT 'generic',
    event_id    TEXT NOT NULL DEFAULT '',
    event_type  TEXT NOT NULL DEFAULT '',
    org_id      UUID,
    status      TEXT NOT NULL DEFAULT 'received',
    raw_payload TEXT NOT NULL DEFAULT '',
    got_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, event_id)
);

CREATE INDEX IF NOT EXISTS idx_billing_webhook_events_time ON billing_webhook_events (got_at);

CREATE TABLE IF NOT EXISTS billing_settings (
    id                 BOOLEAN PRIMARY KEY DEFAULT TRUE,
    webhook_secret     TEXT NOT NULL DEFAULT '',
    external_processor TEXT NOT NULL DEFAULT 'none',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A generated placeholder secret so the receiver is verifiable out of the box;
-- production operators rotate it once live charging processors are attached.
INSERT INTO billing_settings (id, webhook_secret, external_processor)
VALUES (TRUE, 'replace-me-rotate-via-admin-api', 'none')
ON CONFLICT (id) DO NOTHING;