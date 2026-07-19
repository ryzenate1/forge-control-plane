CREATE TABLE IF NOT EXISTS timeline_events (
    id uuid PRIMARY KEY,
    event_id uuid,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    event_type text NOT NULL,
    correlation_id text NOT NULL,
    source text NOT NULL DEFAULT 'api',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_timeline_events_created_at ON timeline_events (created_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_timeline_events_event_id ON timeline_events (event_id) WHERE event_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_timeline_events_resource ON timeline_events (resource_type, resource_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_events_correlation ON timeline_events (correlation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_events_type ON timeline_events (event_type, created_at DESC);

CREATE TABLE IF NOT EXISTS node_heartbeat_history (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL DEFAULT now(),
    previous_seen_at timestamptz,
    gap_seconds integer,
    success boolean NOT NULL DEFAULT true,
    failure_reason text NOT NULL DEFAULT '',
    version text,
    os text,
    architecture text,
    cpu_threads integer,
    memory_mb integer,
    disk_mb integer,
    runtime_status text
);

CREATE INDEX IF NOT EXISTS idx_node_heartbeat_history_node ON node_heartbeat_history (node_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_heartbeat_history_success ON node_heartbeat_history (success, observed_at DESC);

CREATE TABLE IF NOT EXISTS node_health_history (
    id uuid PRIMARY KEY,
    node_id uuid NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL DEFAULT now(),
    actual_state text NOT NULL,
    desired_state text NOT NULL,
    health_score integer NOT NULL DEFAULT 0,
    cpu_score integer NOT NULL DEFAULT 0,
    memory_score integer NOT NULL DEFAULT 0,
    disk_score integer NOT NULL DEFAULT 0,
    heartbeat_score integer NOT NULL DEFAULT 0,
    status_score integer NOT NULL DEFAULT 0,
    allocated_cpu integer NOT NULL DEFAULT 0,
    available_cpu integer NOT NULL DEFAULT 0,
    allocated_memory integer NOT NULL DEFAULT 0,
    available_memory integer NOT NULL DEFAULT 0,
    allocated_disk integer NOT NULL DEFAULT 0,
    available_disk integer NOT NULL DEFAULT 0,
    server_count integer NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_node_health_history_node ON node_health_history (node_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_health_history_state ON node_health_history (actual_state, observed_at DESC);
