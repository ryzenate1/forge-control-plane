ALTER TABLE recovery_tokens
    ADD COLUMN IF NOT EXISTS token_lookup TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS recovery_tokens_lookup_idx
    ON recovery_tokens (token_lookup)
    WHERE token_lookup <> '';
