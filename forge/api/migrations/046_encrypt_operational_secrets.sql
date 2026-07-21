-- Phase 5 encryption-at-rest. Ciphertext envelopes are produced by the API so
-- keys never enter PostgreSQL. Legacy plaintext columns remain temporarily as
-- empty compatibility/rollback targets; the transactional application migrator
-- writes and verifies ciphertext before clearing them.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS daemon_token_encrypted TEXT;
ALTER TABLE database_hosts ADD COLUMN IF NOT EXISTS password_encrypted TEXT;
ALTER TABLE server_databases ADD COLUMN IF NOT EXISTS password_encrypted TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret_encrypted TEXT;
ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS secret_encrypted TEXT;
ALTER TABLE panel_settings
    ADD COLUMN IF NOT EXISTS smtp_password_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS recaptcha_secret_key_encrypted TEXT;
ALTER TABLE panel_mail_settings ADD COLUMN IF NOT EXISTS smtp_password_encrypted TEXT;
ALTER TABLE panel_advanced_settings ADD COLUMN IF NOT EXISTS recaptcha_secret_key_encrypted TEXT;
ALTER TABLE panel_settings_expanded
    ADD COLUMN IF NOT EXISTS discord_webhook_url_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS slack_webhook_url_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS telegram_bot_token_encrypted TEXT;

-- Recovery codes are bcrypt hashes with an independent salt embedded in each
-- hash. Existing plaintext values are converted by the application migrator.
ALTER TABLE recovery_tokens ADD COLUMN IF NOT EXISTS token_hash TEXT;
CREATE INDEX IF NOT EXISTS recovery_tokens_user_hash_idx ON recovery_tokens (user_id) WHERE token_hash IS NOT NULL;
