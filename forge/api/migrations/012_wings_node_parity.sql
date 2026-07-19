ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS uuid UUID,
    ADD COLUMN IF NOT EXISTS fqdn TEXT,
    ADD COLUMN IF NOT EXISTS scheme TEXT NOT NULL DEFAULT 'http',
    ADD COLUMN IF NOT EXISTS behind_proxy BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS maintenance_mode BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS memory_mb INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS memory_overallocate INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS disk_mb INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS disk_overallocate INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upload_size_mb INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN IF NOT EXISTS daemon_base TEXT NOT NULL DEFAULT '/var/lib/forge/volumes',
    ADD COLUMN IF NOT EXISTS daemon_listen INTEGER NOT NULL DEFAULT 8080,
    ADD COLUMN IF NOT EXISTS daemon_sftp INTEGER NOT NULL DEFAULT 2022,
    ADD COLUMN IF NOT EXISTS daemon_token_id TEXT,
    ADD COLUMN IF NOT EXISTS daemon_token TEXT;

UPDATE nodes
SET uuid = COALESCE(uuid, id),
    fqdn = COALESCE(
        fqdn,
        NULLIF(regexp_replace(base_url, '^https?://([^/:]+).*$','\1'), base_url),
        base_url
    ),
    scheme = CASE WHEN base_url LIKE 'https://%' THEN 'https' ELSE scheme END,
    daemon_listen = COALESCE(
        NULLIF(regexp_replace(base_url, '^https?://[^/:]+:([0-9]+).*$','\1'), base_url)::integer,
        daemon_listen
    )
WHERE uuid IS NULL OR fqdn IS NULL;

UPDATE nodes
SET daemon_token_id = COALESCE(daemon_token_id, substring(replace(id::text, '-', '') from 1 for 16)),
    daemon_token = COALESCE(daemon_token, token_hash);

CREATE UNIQUE INDEX IF NOT EXISTS nodes_uuid_idx ON nodes (uuid);
CREATE UNIQUE INDEX IF NOT EXISTS nodes_daemon_token_id_idx ON nodes (daemon_token_id);
