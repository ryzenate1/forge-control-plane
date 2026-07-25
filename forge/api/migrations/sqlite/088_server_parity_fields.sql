-- 088_server_parity_fields.sql (SQLite dialect)
--
-- Why an override is required (the canonical file cannot reach these shapes on
-- its own, because sqliteCompatibleMigration rewrites every bare UUID/JSONB
-- word, including the one used as a COLUMN NAME):
--
--   canonical: ALTER TABLE servers ADD COLUMN IF NOT EXISTS uuid VARCHAR(36);
--   translated: ADD COLUMN IF NOT EXISTS text VARCHAR(36)   <-- wrong column name
--
-- Quoting the identifier protects it: the translator copies double-quoted
-- segments verbatim, so "uuid" stays the column name "uuid" on SQLite exactly
-- as PostgreSQL folds it.
--
-- uuid_short is declared TEXT here, not VARCHAR(8) as the canonical text
-- says, because PostgreSQL never applies 088's declaration: 007_postgres_core
-- _foundation.sql already created servers.uuid_short as TEXT and 088 is
-- "ADD COLUMN IF NOT EXISTS", so on production the effective type is TEXT.
-- Declaring VARCHAR(8) on SQLite would build a different schema than the one
-- production actually converges on, which is what the duplicate-column drift
-- check is there to catch.
ALTER TABLE servers ADD COLUMN IF NOT EXISTS "uuid" VARCHAR(36);
ALTER TABLE servers ADD COLUMN IF NOT EXISTS uuid_short TEXT;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS installed_at TIMESTAMPTZ;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS skip_scripts BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS docker_labels JSONB NOT NULL DEFAULT '{}'::jsonb;
