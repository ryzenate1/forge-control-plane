ALTER TABLE git_credentials
    ADD COLUMN IF NOT EXISTS credential_plaintext TEXT NOT NULL DEFAULT '';

ALTER TABLE git_provider_tokens
    ADD COLUMN IF NOT EXISTS access_token_plaintext TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS refresh_token_plaintext TEXT NOT NULL DEFAULT '';

ALTER TABLE git_sources
    ADD COLUMN IF NOT EXISTS webhook_secret_plaintext TEXT NOT NULL DEFAULT '';
