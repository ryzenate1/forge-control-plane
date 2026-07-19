-- Parity schema: add tables and columns that exist in Pelican but were missing.
-- All statements use IF NOT EXISTS / IF NOT EXISTS for idempotency.

-- 1) backup_hosts table + pivot + FK on backups
CREATE TABLE IF NOT EXISTS backup_hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    schema VARCHAR(255) NOT NULL,
    configuration JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backup_host_node (
    backup_host_id UUID REFERENCES backup_hosts(id) ON DELETE CASCADE,
    node_id UUID REFERENCES nodes(id) ON DELETE CASCADE,
    PRIMARY KEY (backup_host_id, node_id)
);

ALTER TABLE backups ADD COLUMN IF NOT EXISTS backup_host_id UUID REFERENCES backup_hosts(id);

-- 2) node_role pivot table (RBAC)
CREATE TABLE IF NOT EXISTS node_role (
    node_id UUID REFERENCES nodes(id) ON DELETE CASCADE,
    role_id UUID REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (node_id, role_id)
);

-- 3) database_host_node pivot table (M:N)
CREATE TABLE IF NOT EXISTS database_host_node (
    database_host_id UUID REFERENCES database_hosts(id) ON DELETE CASCADE,
    node_id UUID REFERENCES nodes(id) ON DELETE CASCADE,
    PRIMARY KEY (database_host_id, node_id)
);

-- 4) Missing columns on users
ALTER TABLE users ADD COLUMN IF NOT EXISTS language VARCHAR(5) DEFAULT 'en';
ALTER TABLE users ADD COLUMN IF NOT EXISTS timezone VARCHAR(255) DEFAULT 'UTC';
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_managed_externally BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS customization JSONB DEFAULT '{}';

-- 5) is_locked on allocations
ALTER TABLE allocations ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE;
