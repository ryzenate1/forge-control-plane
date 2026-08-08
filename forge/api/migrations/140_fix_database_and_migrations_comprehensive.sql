-- Migration 140: Fix Database & Migrations (Constraints, Indexes, Types, FTS, FKs, Cascades)

-- 1) Enum Types & CHECK Constraints
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'server_status_enum') THEN
        CREATE TYPE server_status_enum AS ENUM ('provisioning', 'running', 'stopped', 'installing', 'suspended', 'transferring', 'error');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'backup_status_enum') THEN
        CREATE TYPE backup_status_enum AS ENUM ('pending', 'processing', 'completed', 'failed');
    END IF;
END $$;

-- 2) Missing Foreign Key Constraints & ON DELETE Actions
ALTER TABLE servers DROP CONSTRAINT IF EXISTS fk_servers_node;
ALTER TABLE servers ADD CONSTRAINT fk_servers_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE servers DROP CONSTRAINT IF EXISTS fk_servers_owner;
ALTER TABLE servers ADD CONSTRAINT fk_servers_owner FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE allocations DROP CONSTRAINT IF EXISTS fk_allocations_node;
ALTER TABLE allocations ADD CONSTRAINT fk_allocations_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE;

ALTER TABLE allocations DROP CONSTRAINT IF EXISTS fk_allocations_server;
ALTER TABLE allocations ADD CONSTRAINT fk_allocations_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE SET NULL;

ALTER TABLE backups DROP CONSTRAINT IF EXISTS fk_backups_server;
ALTER TABLE backups ADD CONSTRAINT fk_backups_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'user_roles' AND column_name = 'role_id') THEN
        ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_role;
        ALTER TABLE user_roles ADD CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
        ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_user;
        ALTER TABLE user_roles ADD CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
END $$;

-- 3) Missing Indexes & Composite Indexes
CREATE INDEX IF NOT EXISTS idx_servers_node_id ON servers (node_id);
CREATE INDEX IF NOT EXISTS idx_servers_owner_id ON servers (owner_id);
CREATE INDEX IF NOT EXISTS idx_servers_node_status ON servers (node_id, status);
CREATE INDEX IF NOT EXISTS idx_allocations_node_id ON allocations (node_id);
CREATE INDEX IF NOT EXISTS idx_allocations_server_id ON allocations (server_id);
CREATE INDEX IF NOT EXISTS idx_allocations_node_server ON allocations (node_id, server_id);
CREATE INDEX IF NOT EXISTS idx_backups_server_id ON backups (server_id);
CREATE INDEX IF NOT EXISTS idx_backups_server_status ON backups (server_id, status);
CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users (lower(email));

-- 4) Full-Text Search GIN Indexes
CREATE INDEX IF NOT EXISTS idx_servers_fts ON servers USING gin(to_tsvector('english', name || ' ' || COALESCE(description, '')));
CREATE INDEX IF NOT EXISTS idx_users_fts ON users USING gin(to_tsvector('english', email || ' ' || COALESCE(username, '')));

-- 5) Data Type & Check Validation Rules
ALTER TABLE allocations DROP CONSTRAINT IF EXISTS allocations_port_range_check;
ALTER TABLE allocations ADD CONSTRAINT allocations_port_range_check CHECK (port BETWEEN 1 AND 65535);

-- 6) Soft Delete Column Standardization
ALTER TABLE servers ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_servers_deleted_at ON servers (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;
