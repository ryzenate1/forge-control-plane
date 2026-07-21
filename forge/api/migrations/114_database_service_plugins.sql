-- Database Service Plugin System
-- Allows users to provision managed databases (PostgreSQL, MySQL, Redis, MongoDB, MariaDB)
-- as standalone Docker containers managed by the panel.

CREATE TABLE IF NOT EXISTS service_templates (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('postgresql','mysql','redis','mongodb','mariadb')),
    version TEXT NOT NULL,
    docker_image TEXT NOT NULL,
    default_port INTEGER NOT NULL,
    default_database TEXT NOT NULL DEFAULT '',
    min_memory_mb INTEGER NOT NULL DEFAULT 256,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (type, version)
);

CREATE TABLE IF NOT EXISTS database_services (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL CHECK (type IN ('postgresql','mysql','redis','mongodb','mariadb')),
    version TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'provisioning' CHECK (status IN ('provisioning','running','stopped','failed','deleting')),
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    username TEXT NOT NULL DEFAULT '',
    encrypted_password TEXT NOT NULL DEFAULT '',
    database_name TEXT NOT NULL DEFAULT '',
    container_id TEXT NOT NULL DEFAULT '',
    volume_id TEXT NOT NULL DEFAULT '',
    memory_mb INTEGER NOT NULL DEFAULT 256,
    cpu_shares INTEGER NOT NULL DEFAULT 0,
    server_id UUID,
    connection_string TEXT NOT NULL DEFAULT '',
    credentials JSONB,
    template_id UUID REFERENCES service_templates(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS database_services_server_id_idx ON database_services (server_id);
CREATE INDEX IF NOT EXISTS database_services_type_idx ON database_services (type);
CREATE INDEX IF NOT EXISTS database_services_status_idx ON database_services (status);

CREATE TABLE IF NOT EXISTS database_service_backups (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES database_services(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed')),
    file_path TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS database_service_backups_service_id_idx ON database_service_backups (service_id);

CREATE TABLE IF NOT EXISTS database_service_credentials (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES database_services(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    encrypted_password TEXT NOT NULL DEFAULT '',
    database_name TEXT NOT NULL DEFAULT '',
    permissions TEXT NOT NULL DEFAULT 'read-write' CHECK (permissions IN ('read-only','read-write','admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS database_service_credentials_service_id_idx ON database_service_credentials (service_id);

-- Seed default service templates
INSERT INTO service_templates (id, type, version, docker_image, default_port, default_database, min_memory_mb) VALUES
    (gen_random_uuid(), 'postgresql', '16', 'postgres:16', 5432, 'postgres', 256),
    (gen_random_uuid(), 'postgresql', '15', 'postgres:15', 5432, 'postgres', 256),
    (gen_random_uuid(), 'mysql', '8.3', 'mysql:8.3', 3306, 'mysql', 256),
    (gen_random_uuid(), 'mysql', '8.0', 'mysql:8.0', 3306, 'mysql', 256),
    (gen_random_uuid(), 'mariadb', '11', 'mariadb:11', 3306, 'mysql', 256),
    (gen_random_uuid(), 'mariadb', '10', 'mariadb:10', 3306, 'mysql', 256),
    (gen_random_uuid(), 'redis', '7', 'redis:7', 6379, '', 128),
    (gen_random_uuid(), 'redis', '6', 'redis:6', 6379, '', 128),
    (gen_random_uuid(), 'mongodb', '7', 'mongo:7', 27017, 'admin', 256),
    (gen_random_uuid(), 'mongodb', '6', 'mongo:6', 27017, 'admin', 256)
ON CONFLICT (type, version) DO NOTHING;
