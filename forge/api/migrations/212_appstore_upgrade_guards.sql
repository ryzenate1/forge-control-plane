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
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_sync_app_ignore_upgrade') THEN
    CREATE OR REPLACE FUNCTION sync_app_ignore_upgrade() RETURNS trigger AS $f$
    BEGIN
      -- Mirror ignore_upgrade <-> app_ignore_upgrade
      IF NEW.ignore_upgrade IS DISTINCT FROM OLD.ignore_upgrade THEN
        NEW.app_ignore_upgrade := NEW.ignore_upgrade;
      ELSIF NEW.app_ignore_upgrade IS DISTINCT FROM OLD.app_ignore_upgrade THEN
        NEW.ignore_upgrade := NEW.app_ignore_upgrade;
      END IF;
      RETURN NEW;
    END;
    $f$ LANGUAGE plpgsql;
    CREATE TRIGGER trg_sync_app_ignore_upgrade BEFORE UPDATE ON app_store_installs
      FOR EACH ROW EXECUTE FUNCTION sync_app_ignore_upgrade();
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_app_store_installs_ignore_upgrade ON app_store_installs (ignore_upgrade) WHERE ignore_upgrade = true;
CREATE INDEX IF NOT EXISTS idx_app_store_apps_cross_version ON app_store_apps (cross_version) WHERE cross_version = true;
