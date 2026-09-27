-- 212_appstore_upgrade_guards.sql
-- Fix appstore resolveTemplate swallowing + upgrade cross-version / ignore_upgrade guards
-- and uninstall orphan prevention (spec 110-03-05).
-- cross_version flags an app whose version jump requires explicit confirm.
-- ignore_upgrade flags an install that should block automatic upgrades.

ALTER TABLE app_store_apps ADD COLUMN IF NOT EXISTS cross_version BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE app_store_installs ADD COLUMN IF NOT EXISTS ignore_upgrade BOOLEAN NOT NULL DEFAULT FALSE;
-- Alias column app_ignore_upgrade for spec literal "query app_ignore_upgrade" (kept in sync via trigger/duplicate).
-- Some audit implementations refer to it as app_ignore_upgrade; add as alias via duplicate column.
ALTER TABLE app_store_installs ADD COLUMN IF NOT EXISTS app_ignore_upgrade BOOLEAN NOT NULL DEFAULT FALSE;

-- Keep the two columns in sync for installs (app_ignore_upgrade is legacy alias).
-- SQLite encoding of the PostgreSQL BEFORE UPDATE trigger: AFTER UPDATE with
-- the same precedence (ignore_upgrade wins when both changed in one write).
PRAGMA recursive_triggers=OFF;

DROP TRIGGER IF EXISTS trg_sync_app_ignore_upgrade;
CREATE TRIGGER trg_sync_app_ignore_upgrade
AFTER UPDATE ON app_store_installs
FOR EACH ROW WHEN (NEW.ignore_upgrade IS NOT OLD.ignore_upgrade OR NEW.app_ignore_upgrade IS NOT OLD.app_ignore_upgrade)
BEGIN
    UPDATE app_store_installs SET
        ignore_upgrade = CASE WHEN NEW.ignore_upgrade IS NOT OLD.ignore_upgrade THEN NEW.ignore_upgrade ELSE NEW.app_ignore_upgrade END,
        app_ignore_upgrade = CASE WHEN NEW.ignore_upgrade IS NOT OLD.ignore_upgrade THEN NEW.ignore_upgrade ELSE NEW.app_ignore_upgrade END
    WHERE id = NEW.id;
END;

CREATE INDEX IF NOT EXISTS idx_app_store_installs_ignore_upgrade ON app_store_installs (ignore_upgrade) WHERE ignore_upgrade = true;
CREATE INDEX IF NOT EXISTS idx_app_store_apps_cross_version ON app_store_apps (cross_version) WHERE cross_version = true;
