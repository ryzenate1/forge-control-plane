CREATE TABLE IF NOT EXISTS users (
    id CHAR(36) PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS nodes (
    id CHAR(36) PRIMARY KEY,
    name TEXT NOT NULL,
    region TEXT NOT NULL,
    base_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline',
    token_hash TEXT NOT NULL,
    last_seen_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS server_templates (
    id CHAR(36) PRIMARY KEY,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    startup_command TEXT NOT NULL,
    default_memory_mb INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS servers (
    id CHAR(36) PRIMARY KEY,
    node_id CHAR(36) NOT NULL,
    owner_id CHAR(36) NOT NULL,
    template_id CHAR(36) NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    memory_mb INTEGER NOT NULL,
    cpu_shares INTEGER NOT NULL,
    disk_mb INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id),
    FOREIGN KEY (owner_id) REFERENCES users(id),
    FOREIGN KEY (template_id) REFERENCES server_templates(id)
);

CREATE TABLE IF NOT EXISTS allocations (
    id CHAR(36) PRIMARY KEY,
    node_id CHAR(36) NOT NULL,
    server_id CHAR(36) NULL,
    ip TEXT NOT NULL,
    port INTEGER NOT NULL,
    alias TEXT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (node_id, ip, port),
    FOREIGN KEY (node_id) REFERENCES nodes(id),
    FOREIGN KEY (server_id) REFERENCES servers(id)
);

CREATE TABLE IF NOT EXISTS audit_events (
    id CHAR(36) PRIMARY KEY,
    actor_id CHAR(36) NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id CHAR(36) NULL,
    metadata JSON NOT NULL DEFAULT ('{}'),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (actor_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events (created_at DESC);
CREATE INDEX IF NOT EXISTS servers_node_id_idx ON servers (node_id);
CREATE INDEX IF NOT EXISTS servers_owner_id_idx ON servers (owner_id);
CREATE INDEX IF NOT EXISTS allocations_node_id_idx ON allocations (node_id);
CREATE INDEX IF NOT EXISTS allocations_server_id_idx ON allocations (server_id);
