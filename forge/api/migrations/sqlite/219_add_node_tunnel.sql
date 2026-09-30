-- 219_add_node_tunnel.sql (SQLite dialect)
-- Mirrors pg migration 219_add_node_tunnel.sql with SQLite-compatible types.
-- INET is mapped to TEXT for SQLite (sqliteCompatibleMigration also handles this,
-- but explicit dialect file ensures clarity). IF NOT EXISTS is supported via
-- runner's duplicate column handling; plain ADD COLUMN is used for max compatibility.
ALTER TABLE nodes ADD COLUMN tunnel_ip TEXT;
ALTER TABLE nodes ADD COLUMN mesh_pubkey TEXT;
