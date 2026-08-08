-- Rollback Migration 140: Fix Database & Migrations

DROP INDEX IF EXISTS idx_users_deleted_at;
DROP INDEX IF EXISTS idx_servers_deleted_at;

ALTER TABLE nodes DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE servers DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_users_fts;
DROP INDEX IF EXISTS idx_servers_fts;

DROP INDEX IF EXISTS idx_users_email_lower;
DROP INDEX IF EXISTS idx_backups_server_status;
DROP INDEX IF EXISTS idx_backups_server_id;
DROP INDEX IF EXISTS idx_allocations_node_server;
DROP INDEX IF EXISTS idx_allocations_server_id;
DROP INDEX IF EXISTS idx_allocations_node_id;
DROP INDEX IF EXISTS idx_servers_node_status;
DROP INDEX IF EXISTS idx_servers_owner_id;
DROP INDEX IF EXISTS idx_servers_node_id;

ALTER TABLE backups DROP CONSTRAINT IF EXISTS fk_backups_server;
ALTER TABLE allocations DROP CONSTRAINT IF EXISTS fk_allocations_server;
ALTER TABLE allocations DROP CONSTRAINT IF EXISTS fk_allocations_node;
ALTER TABLE servers DROP CONSTRAINT IF EXISTS fk_servers_owner;
ALTER TABLE servers DROP CONSTRAINT IF EXISTS fk_servers_node;

DROP TYPE IF EXISTS backup_status_enum;
DROP TYPE IF EXISTS server_status_enum;
