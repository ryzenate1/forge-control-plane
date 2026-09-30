ALTER TABLE acme_accounts
    ADD COLUMN IF NOT EXISTS private_key_encrypted TEXT NOT NULL DEFAULT '';
