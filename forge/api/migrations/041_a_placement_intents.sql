CREATE TABLE IF NOT EXISTS placement_intents (
    id TEXT PRIMARY KEY,
    server_id TEXT,
    node_id TEXT NOT NULL,
    allocation_id TEXT,
    reservation_id TEXT,
    cpu INT NOT NULL DEFAULT 0,
    memory_mb INT NOT NULL DEFAULT 0,
    disk_mb INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_placement_intents_status ON placement_intents(status);
CREATE INDEX IF NOT EXISTS idx_placement_intents_node_id ON placement_intents(node_id);
CREATE INDEX IF NOT EXISTS idx_placement_intents_server_id ON placement_intents(server_id);
