-- Mail settings (`mail:mailers:smtp:*` and `mail:from:*`).
-- Stored as a single-row table for simplicity, since the panel only has one
-- mail configuration at a time.

CREATE TABLE IF NOT EXISTS panel_mail_settings (
    id                    BOOLEAN PRIMARY KEY DEFAULT TRUE,
    smtp_host             TEXT NOT NULL DEFAULT '',
    smtp_port             INTEGER NOT NULL DEFAULT 587,
    smtp_encryption       TEXT NOT NULL DEFAULT 'tls',  -- '' | 'tls' | 'ssl'
    smtp_username         TEXT NOT NULL DEFAULT '',
    smtp_password         TEXT NOT NULL DEFAULT '',
    mail_from_address     TEXT NOT NULL DEFAULT '',
    mail_from_name        TEXT NOT NULL DEFAULT '',
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT panel_mail_settings_single CHECK (id)
);

INSERT INTO panel_mail_settings (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;

-- Advanced settings.
--   - reCAPTCHA credentials
--   - HTTP connection/request timeouts
--   - Automatic allocation creation range

CREATE TABLE IF NOT EXISTS panel_advanced_settings (
    id                          BOOLEAN PRIMARY KEY DEFAULT TRUE,
    recaptcha_enabled           BOOLEAN NOT NULL DEFAULT FALSE,
    recaptcha_website_key       TEXT NOT NULL DEFAULT '',
    recaptcha_secret_key        TEXT NOT NULL DEFAULT '',
    guzzle_connect_timeout      INTEGER NOT NULL DEFAULT 30,
    guzzle_request_timeout      INTEGER NOT NULL DEFAULT 30,
    auto_alloc_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    auto_alloc_start_port       INTEGER NOT NULL DEFAULT 25565,
    auto_alloc_end_port         INTEGER NOT NULL DEFAULT 25600,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT panel_advanced_settings_single CHECK (id)
);

INSERT INTO panel_advanced_settings (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;
