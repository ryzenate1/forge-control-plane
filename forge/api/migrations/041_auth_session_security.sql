-- Authentication/session hardening.
-- The version is checked against every JWT/OAuth request. Password and 2FA
-- changes increment it to revoke all previously issued session tokens.
-- Roles are intentionally not copied into this revision: middleware reloads the
-- effective role from user_roles/roles on every request, so assignment changes
-- take effect immediately without forcing a token refresh.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS session_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT FALSE;
