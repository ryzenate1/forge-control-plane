CREATE TABLE IF NOT EXISTS replica_applications (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    replicas INTEGER NOT NULL DEFAULT 1,
    cpu INTEGER NOT NULL DEFAULT 1024,
    memory_mb INTEGER NOT NULL DEFAULT 2048,
    disk_mb INTEGER NOT NULL DEFAULT 10240,
    runtime_provider TEXT NOT NULL DEFAULT 'docker',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_replica_applications_name ON replica_applications (name);

CREATE TABLE IF NOT EXISTS instances (
    id UUID PRIMARY KEY,
    app_id UUID NOT NULL REFERENCES replica_applications(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    node_id UUID NOT NULL REFERENCES nodes(id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','provisioning','running','stopped','failed','removing')),
    cpu INTEGER NOT NULL DEFAULT 1024,
    memory_mb INTEGER NOT NULL DEFAULT 2048,
    disk_mb INTEGER NOT NULL DEFAULT 10240,
    allocation_id UUID REFERENCES allocations(id),
    placement_id UUID NOT NULL,
    reservation_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT 'docker',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (app_id, idx)
);
CREATE INDEX IF NOT EXISTS idx_instances_app ON instances (app_id, idx);
CREATE INDEX IF NOT EXISTS idx_instances_node ON instances (node_id);
CREATE INDEX IF NOT EXISTS idx_instances_status ON instances (status);

CREATE TABLE IF NOT EXISTS placement_decisions (
    id UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id),
    app_id UUID NOT NULL REFERENCES replica_applications(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    accepted BOOLEAN NOT NULL DEFAULT true,
    reasons TEXT[] NOT NULL DEFAULT '{}',
    runtime_provider TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_placement_decisions_instance ON placement_decisions (instance_id);
CREATE INDEX IF NOT EXISTS idx_placement_decisions_node ON placement_decisions (node_id);
CREATE INDEX IF NOT EXISTS idx_placement_decisions_app ON placement_decisions (app_id);
