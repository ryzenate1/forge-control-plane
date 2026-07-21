-- Dokploy-style backup schedules and orchestration
-- Adds workload-aware backup policies, database/volume backup support,
-- backup manifests, storage receipts, and retention enhancements.

-- 1. Extend backup_policies with workload-awareness
ALTER TABLE backup_policies
    ADD COLUMN IF NOT EXISTS app_id TEXT,
    ADD COLUMN IF NOT EXISTS service_id TEXT,
    ADD COLUMN IF NOT EXISTS database_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS volume_backup BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS database_id TEXT;

CREATE INDEX IF NOT EXISTS idx_backup_policies_app ON backup_policies (app_id) WHERE app_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_backup_policies_service ON backup_policies (service_id) WHERE service_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_backup_policies_database ON backup_policies (database_id) WHERE database_id IS NOT NULL;

-- 2. Extend backups with manifest, storage receipt, workload fields
ALTER TABLE backups
    ADD COLUMN IF NOT EXISTS manifest JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS storage_receipt JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'server',
    ADD COLUMN IF NOT EXISTS source_id TEXT,
    ADD COLUMN IF NOT EXISTS database_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS volume_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS checksum_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS restore_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_restore_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_backups_source ON backups (source_type, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_backups_database_type ON backups (database_type) WHERE database_type != '';

-- 3. Backup manifest and checksum records
CREATE TABLE IF NOT EXISTS backup_manifests (
    id UUID PRIMARY KEY,
    backup_id UUID NOT NULL REFERENCES backups(uuid) ON DELETE CASCADE,
    manifest_version INTEGER NOT NULL DEFAULT 1,
    checksum_algorithm TEXT NOT NULL DEFAULT 'sha256',
    checksum_value TEXT NOT NULL DEFAULT '',
    file_count INTEGER NOT NULL DEFAULT 0,
    total_size_bytes BIGINT NOT NULL DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (backup_id)
);

CREATE INDEX IF NOT EXISTS idx_backup_manifests_backup ON backup_manifests (backup_id);

-- 4. Storage receipts for remote storage verification
CREATE TABLE IF NOT EXISTS backup_storage_receipts (
    id UUID PRIMARY KEY,
    backup_id UUID NOT NULL REFERENCES backups(uuid) ON DELETE CASCADE,
    storage_adapter TEXT NOT NULL DEFAULT '',
    storage_path TEXT NOT NULL DEFAULT '',
    storage_etag TEXT NOT NULL DEFAULT '',
    storage_version_id TEXT NOT NULL DEFAULT '',
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at TIMESTAMPTZ,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    receipt_data JSONB DEFAULT '{}',
    UNIQUE (backup_id)
);

CREATE INDEX IF NOT EXISTS idx_backup_storage_receipts_backup ON backup_storage_receipts (backup_id);

-- 5. Database backup records for DB-aware backups
CREATE TABLE IF NOT EXISTS database_backups (
    id UUID PRIMARY KEY,
    backup_id UUID NOT NULL REFERENCES backups(uuid) ON DELETE CASCADE,
    database_id TEXT NOT NULL,
    engine TEXT NOT NULL DEFAULT '',
    database_name TEXT NOT NULL DEFAULT '',
    dump_format TEXT NOT NULL DEFAULT 'sql',
    compression TEXT NOT NULL DEFAULT 'gzip',
    options JSONB DEFAULT '{}',
    UNIQUE (backup_id)
);

CREATE INDEX IF NOT EXISTS idx_database_backups_db ON database_backups (database_id);

-- 6. Volume backup records
CREATE TABLE IF NOT EXISTS volume_backups (
    id UUID PRIMARY KEY,
    backup_id UUID NOT NULL REFERENCES backups(uuid) ON DELETE CASCADE,
    volume_name TEXT NOT NULL DEFAULT '',
    volume_mount_path TEXT NOT NULL DEFAULT '',
    snapshot_id TEXT NOT NULL DEFAULT '',
    include_paths TEXT[] NOT NULL DEFAULT '{}',
    exclude_paths TEXT[] NOT NULL DEFAULT '{}',
    UNIQUE (backup_id)
);

CREATE INDEX IF NOT EXISTS idx_volume_backups_volume ON volume_backups (volume_name);

-- 7. Backup schedule cleanup triggers for deleted workloads
CREATE OR REPLACE FUNCTION cleanup_orphan_backup_policies()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM backup_policies
    WHERE (app_id IS NOT NULL AND app_id = OLD.id)
       OR (database_id IS NOT NULL AND database_id = OLD.id);
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Note: trigger attachment is deferred to app-level for flexibility
