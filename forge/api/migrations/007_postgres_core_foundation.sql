-- Core PostgreSQL foundation for the panel data model.
-- This migration is additive and backward-compatible with the current app code.

-- 1) Roles and rule sets (RBAC base).
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS role_rules (
    id UUID PRIMARY KEY,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    rule_key TEXT NOT NULL,
    effect TEXT NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (role_id, rule_key)
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

-- Seed basic roles if missing.
INSERT INTO roles (id, key, name, is_admin)
VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'admin', 'Administrator', TRUE),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'user', 'User', FALSE)
ON CONFLICT (key) DO NOTHING;

-- Backfill user_roles from existing users.role values.
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.key = COALESCE(NULLIF(lower(u.role), ''), 'user')
ON CONFLICT (user_id, role_id) DO NOTHING;

-- 2) User/account shape improvements.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS username TEXT,
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS first_name TEXT,
    ADD COLUMN IF NOT EXISTS last_name TEXT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE users
SET username = COALESCE(NULLIF(username, ''), split_part(email, '@', 1))
WHERE username IS NULL OR username = '';

CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique ON users (username);
CREATE UNIQUE INDEX IF NOT EXISTS users_external_id_unique ON users (external_id) WHERE external_id IS NOT NULL;

-- 3) Location hierarchy for nodes.
CREATE TABLE IF NOT EXISTS locations (
    id UUID PRIMARY KEY,
    short TEXT NOT NULL UNIQUE,
    long TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO locations (id, short, long)
VALUES ('cccccccc-cccc-cccc-cccc-cccccccccccc', 'local', 'Local Lab')
ON CONFLICT (short) DO NOTHING;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS location_id UUID REFERENCES locations(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS fqdn TEXT,
    ADD COLUMN IF NOT EXISTS daemon_listen_port INTEGER NOT NULL DEFAULT 8080 CHECK (daemon_listen_port BETWEEN 1 AND 65535),
    ADD COLUMN IF NOT EXISTS daemon_sftp_port INTEGER NOT NULL DEFAULT 2022 CHECK (daemon_sftp_port BETWEEN 1 AND 65535),
    ADD COLUMN IF NOT EXISTS memory_mb INTEGER,
    ADD COLUMN IF NOT EXISTS disk_mb INTEGER,
    ADD COLUMN IF NOT EXISTS upload_size_mb INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN IF NOT EXISTS maintenance_mode BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE nodes
SET location_id = COALESCE(location_id, 'cccccccc-cccc-cccc-cccc-cccccccccccc'::uuid)
WHERE location_id IS NULL;

CREATE INDEX IF NOT EXISTS nodes_location_id_idx ON nodes (location_id);

-- 4) Nest/Egg-like template hierarchy.
CREATE TABLE IF NOT EXISTS nests (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS eggs (
    id UUID PRIMARY KEY,
    nest_id UUID NOT NULL REFERENCES nests(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    docker_images JSONB NOT NULL DEFAULT '[]'::jsonb,
    startup TEXT NOT NULL DEFAULT '',
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (nest_id, name)
);

INSERT INTO nests (id, name, description)
VALUES ('dddddddd-dddd-dddd-dddd-dddddddddddd', 'Games', 'Default game nest')
ON CONFLICT (name) DO NOTHING;

-- 5) Server shape & limits closer to panel behavior.
ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS uuid_short TEXT,
    ADD COLUMN IF NOT EXISTS description TEXT,
    ADD COLUMN IF NOT EXISTS database_limit INTEGER NOT NULL DEFAULT 0 CHECK (database_limit >= 0),
    ADD COLUMN IF NOT EXISTS allocation_limit INTEGER NOT NULL DEFAULT 0 CHECK (allocation_limit >= 0),
    ADD COLUMN IF NOT EXISTS backup_limit INTEGER NOT NULL DEFAULT 0 CHECK (backup_limit >= 0),
    ADD COLUMN IF NOT EXISTS io_weight INTEGER NOT NULL DEFAULT 500 CHECK (io_weight BETWEEN 10 AND 1000),
    ADD COLUMN IF NOT EXISTS swap_mb INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS threads TEXT,
    ADD COLUMN IF NOT EXISTS oom_disabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS installed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS servers_external_id_unique ON servers (external_id) WHERE external_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS servers_uuid_short_unique ON servers (uuid_short) WHERE uuid_short IS NOT NULL;

-- Generate deterministic short IDs for existing rows (first 8 chars, no dashes).
UPDATE servers
SET uuid_short = substr(replace(id::text, '-', ''), 1, 8)
WHERE uuid_short IS NULL;

-- 6) Allocation/IP correctness and indexes.
ALTER TABLE allocations
    ALTER COLUMN ip TYPE inet USING ip::inet;

ALTER TABLE allocations
    ADD COLUMN IF NOT EXISTS ip_alias TEXT,
    ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ;

ALTER TABLE allocations
    DROP CONSTRAINT IF EXISTS allocations_port_range_check;
ALTER TABLE allocations
    ADD CONSTRAINT allocations_port_range_check CHECK (port BETWEEN 1 AND 65535);

CREATE INDEX IF NOT EXISTS allocations_node_ip_port_idx ON allocations (node_id, ip, port);
CREATE INDEX IF NOT EXISTS allocations_unassigned_idx ON allocations (node_id, port) WHERE server_id IS NULL;

-- 7) Subusers + permission JSON base.
CREATE TABLE IF NOT EXISTS subusers (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, user_id)
);

CREATE INDEX IF NOT EXISTS subusers_user_id_idx ON subusers (user_id);

-- 8) Transfer history table (panel-style lineage of attempts).
CREATE TABLE IF NOT EXISTS server_transfers (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    old_node_id UUID NOT NULL REFERENCES nodes(id),
    new_node_id UUID NOT NULL REFERENCES nodes(id),
    old_primary_allocation_id UUID REFERENCES allocations(id),
    new_primary_allocation_id UUID REFERENCES allocations(id),
    old_additional_allocations JSONB,
    new_additional_allocations JSONB,
    successful BOOLEAN,
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS server_transfers_server_id_idx ON server_transfers (server_id);
CREATE INDEX IF NOT EXISTS server_transfers_created_at_idx ON server_transfers (created_at DESC);
