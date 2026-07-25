CREATE TABLE IF NOT EXISTS process_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    process_type VARCHAR(32) NOT NULL CHECK (process_type IN ('web','worker','clock','release')),
    command TEXT NOT NULL DEFAULT '',
    quantity INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(server_id, process_type)
);

CREATE TABLE IF NOT EXISTS process_scaling_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    process_type VARCHAR(32) NOT NULL,
    old_quantity INT NOT NULL,
    new_quantity INT NOT NULL,
    triggered_by VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS one_off_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    command TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    output TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_process_types_server_id ON process_types(server_id);
CREATE INDEX IF NOT EXISTS idx_process_scaling_events_server_id ON process_scaling_events(server_id);
CREATE INDEX IF NOT EXISTS idx_one_off_tasks_server_id ON one_off_tasks(server_id);
