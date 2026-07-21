CREATE TABLE IF NOT EXISTS managed_databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID,
    name VARCHAR(255) NOT NULL,
    engine VARCHAR(50) NOT NULL,
    version VARCHAR(50) NOT NULL DEFAULT 'latest',
    docker_image VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'creating',
    host VARCHAR(255),
    port INT NOT NULL DEFAULT 0,
    username VARCHAR(255),
    password_encrypted TEXT,
    database_name VARCHAR(255),
    memory_mb INT NOT NULL DEFAULT 256,
    cpu_shares INT NOT NULL DEFAULT 0,
    volume_id VARCHAR(255),
    container_id VARCHAR(255),
    connection_string TEXT,
    credentials JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS managed_database_backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    managed_database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    engine VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    size BIGINT NOT NULL DEFAULT 0,
    checksum VARCHAR(255),
    storage_path TEXT,
    storage_adapter VARCHAR(100) DEFAULT 'local',
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS managed_database_restores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    managed_database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE CASCADE,
    backup_id UUID REFERENCES managed_database_backups(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_managed_databases_server_id ON managed_databases(server_id);
CREATE INDEX IF NOT EXISTS idx_managed_databases_status ON managed_databases(status);
CREATE INDEX IF NOT EXISTS idx_managed_database_backups_db_id ON managed_database_backups(managed_database_id);
CREATE INDEX IF NOT EXISTS idx_managed_database_backups_status ON managed_database_backups(status);
CREATE INDEX IF NOT EXISTS idx_managed_database_restores_db_id ON managed_database_restores(managed_database_id);
