-- Intentionally empty. The catalog_attach_links table depends on
-- catalog_instances, which is created in 177_catalog_instances.sql, so the
-- DDL lives there instead. This file is kept so the migration version
-- sequence stays stable for installs that already recorded it.
SELECT 1;
