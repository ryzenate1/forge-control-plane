ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS version TEXT,
    ADD COLUMN IF NOT EXISTS os TEXT,
    ADD COLUMN IF NOT EXISTS architecture TEXT,
    ADD COLUMN IF NOT EXISTS cpu_threads INTEGER,
    ADD COLUMN IF NOT EXISTS docker_status TEXT,
    ADD COLUMN IF NOT EXISTS node_memory_mb INTEGER,
    ADD COLUMN IF NOT EXISTS node_disk_mb INTEGER,
    ADD COLUMN IF NOT EXISTS heartbeat_error TEXT;

CREATE INDEX IF NOT EXISTS nodes_last_seen_at_idx ON nodes (last_seen_at DESC);
