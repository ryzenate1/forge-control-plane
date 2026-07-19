-- Panel-wide settings (replaces the missing /admin/settings 404).
-- Stores all operator-controlled settings as a single key-value row so the
-- settings page can be served from real backend state instead of a fake
-- fallback.

CREATE TABLE IF NOT EXISTS panel_settings (
    id                 BOOLEAN PRIMARY KEY DEFAULT TRUE,
    company_name       TEXT NOT NULL DEFAULT 'Forge Control Plane',
    require_2fa        TEXT NOT NULL DEFAULT 'none',
    default_locale     TEXT NOT NULL DEFAULT 'en',
    smtp_host          TEXT NOT NULL DEFAULT '',
    smtp_port          INTEGER NOT NULL DEFAULT 587,
    smtp_encryption    TEXT NOT NULL DEFAULT 'tls',
    smtp_username      TEXT NOT NULL DEFAULT '',
    smtp_password      TEXT NOT NULL DEFAULT '',
    mail_from_address  TEXT NOT NULL DEFAULT '',
    mail_from_name     TEXT NOT NULL DEFAULT '',
    recaptcha_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    recaptcha_site_key TEXT NOT NULL DEFAULT '',
    recaptcha_secret_key TEXT NOT NULL DEFAULT '',
    connection_timeout INTEGER NOT NULL DEFAULT 30,
    request_timeout    INTEGER NOT NULL DEFAULT 30,
    auto_alloc_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    auto_alloc_start_port INTEGER NOT NULL DEFAULT 25565,
    auto_alloc_end_port   INTEGER NOT NULL DEFAULT 25600,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT panel_settings_single CHECK (id)
);

INSERT INTO panel_settings (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;
