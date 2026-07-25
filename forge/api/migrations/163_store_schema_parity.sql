-- Store/schema parity: reconcile recovery_tokens and users with the columns
-- actually read/written by store code.
--
-- recovery_tokens was originally created (018_a) as a 2FA backup-code table
-- with (id, user_id, token, created_at); 046 added token_hash and 156 added
-- token_lookup. The unified recovery token store
-- (forge/api/internal/services/recovery/store.go) inserts and selects
-- type, expires_at, used_at, metadata, ip, user_agent on the same table.
-- Add those columns idempotently. Existing rows (legacy 2FA backup codes) are
-- classified as 2fa_recovery and left with a NULL expires_at so they keep
-- working and are never expired by CleanupExpiredTokens.
ALTER TABLE recovery_tokens
    ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT '2fa_recovery',
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS used_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS ip INET,
    ADD COLUMN IF NOT EXISTS user_agent TEXT;

CREATE INDEX IF NOT EXISTS idx_recovery_tokens_user
    ON recovery_tokens (user_id, type);
CREATE INDEX IF NOT EXISTS idx_recovery_tokens_expires
    ON recovery_tokens (expires_at) WHERE used_at IS NULL;

-- The legacy token column is NOT NULL but the unified recovery token store
-- (forge/api/internal/services/recovery/store.go) inserts rows without it,
-- which would violate the constraint. Make it nullable; both store paths
-- either supply the value explicitly (2FA backup codes write '') or rely on
-- COALESCE(token, '').
ALTER TABLE recovery_tokens ALTER COLUMN token DROP NOT NULL;

-- users.name_first / name_last are read by the admin user search store
-- (forge/api/internal/store/store_admin_extras.go) but were never added by a
-- canonical migration. Add them with the same defaults as the auth core schema.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS name_first TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS name_last TEXT NOT NULL DEFAULT '';
