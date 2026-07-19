CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    region TEXT NOT NULL,
    base_url TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline',
    token_hash TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS server_templates (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    startup_command TEXT NOT NULL,
    default_memory_mb INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS servers (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id),
    owner_id UUID NOT NULL REFERENCES users(id),
    template_id UUID NOT NULL REFERENCES server_templates(id),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    memory_mb INTEGER NOT NULL,
    cpu_shares INTEGER NOT NULL,
    disk_mb INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS allocations (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id),
    server_id UUID REFERENCES servers(id),
    ip TEXT NOT NULL,
    port INTEGER NOT NULL,
    alias TEXT,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (node_id, ip, port)
);

CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY,
    actor_id UUID REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_events_created_at_idx ON audit_events (created_at DESC);
CREATE INDEX IF NOT EXISTS servers_node_id_idx ON servers (node_id);
CREATE INDEX IF NOT EXISTS servers_owner_id_idx ON servers (owner_id);
CREATE INDEX IF NOT EXISTS allocations_node_id_idx ON allocations (node_id);
CREATE INDEX IF NOT EXISTS allocations_server_id_idx ON allocations (server_id);
