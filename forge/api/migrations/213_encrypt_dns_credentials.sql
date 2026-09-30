-- 213_encrypt_dns_credentials.sql
-- Mirror 157_encrypt_acme pattern: add encrypted envelope column for DNS provider credentials
-- Plaintext JSONB/TEXT columns remain temporarily as compatibility targets; application migrator
-- writes ciphertext before clearing them (dual-write + backfill).
ALTER TABLE certificates ADD COLUMN IF NOT EXISTS dns_credentials_encrypted TEXT NOT NULL DEFAULT '';
ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '';
