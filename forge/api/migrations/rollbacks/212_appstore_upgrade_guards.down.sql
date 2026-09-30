-- Down for 212: remove appstore upgrade guards
DROP TRIGGER IF EXISTS trg_sync_app_ignore_upgrade ON app_store_installs;
DROP FUNCTION IF EXISTS sync_app_ignore_upgrade();
DROP INDEX IF EXISTS idx_app_store_installs_ignore_upgrade;
DROP INDEX IF EXISTS idx_app_store_apps_cross_version;
ALTER TABLE app_store_installs DROP COLUMN IF EXISTS app_ignore_upgrade;
ALTER TABLE app_store_installs DROP COLUMN IF EXISTS ignore_upgrade;
ALTER TABLE app_store_apps DROP COLUMN IF EXISTS cross_version;
