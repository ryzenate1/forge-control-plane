-- Migration 035: Observability & Monitoring
-- Adds node metrics, workload metrics, build/deployment/beacon logs, alerts, notification routes

-- Node metrics time-series
CREATE TABLE IF NOT EXISTS node_metrics (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    disk_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_mb BIGINT NOT NULL DEFAULT 0,
    memory_total_mb BIGINT NOT NULL DEFAULT 0,
    disk_used_mb BIGINT NOT NULL DEFAULT 0,
    disk_total_mb BIGINT NOT NULL DEFAULT 0,
    cpu_load_1m DOUBLE PRECISION DEFAULT 0,
    cpu_load_5m DOUBLE PRECISION DEFAULT 0,
    cpu_load_15m DOUBLE PRECISION DEFAULT 0,
    network_rx_bytes BIGINT DEFAULT 0,
    network_tx_bytes BIGINT DEFAULT 0,
    container_running INT DEFAULT 0,
    container_total INT DEFAULT 0,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_node_ts ON node_metrics(node_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_node_metrics_observed_at ON node_metrics(observed_at DESC);

-- Workload metrics per container/service
CREATE TABLE IF NOT EXISTS workload_metrics (
    id UUID PRIMARY KEY,
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL,
    container_name TEXT NOT NULL DEFAULT '',
    cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_mb BIGINT NOT NULL DEFAULT 0,
    memory_limit_mb BIGINT NOT NULL DEFAULT 0,
    disk_read_bytes BIGINT DEFAULT 0,
    disk_write_bytes BIGINT DEFAULT 0,
    network_rx_bytes BIGINT DEFAULT 0,
    network_tx_bytes BIGINT DEFAULT 0,
    pids INT DEFAULT 0,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workload_metrics_server_ts ON workload_metrics(server_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_workload_metrics_node_ts ON workload_metrics(node_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_workload_metrics_container ON workload_metrics(container_id, observed_at DESC);

-- Build logs persisted with correlation
CREATE TABLE IF NOT EXISTS build_logs (
    id UUID PRIMARY KEY,
    build_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    source_type TEXT NOT NULL DEFAULT 'build',
    log_level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB DEFAULT '{}',
    sequence INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_build_logs_build ON build_logs(build_id, sequence);
CREATE INDEX IF NOT EXISTS idx_build_logs_correlation ON build_logs(correlation_id);
CREATE INDEX IF NOT EXISTS idx_build_logs_created ON build_logs(created_at DESC);

-- Deployment logs persisted with correlation
CREATE TABLE IF NOT EXISTS deployment_logs (
    id UUID PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    log_level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL DEFAULT '',
    metadata JSONB DEFAULT '{}',
    sequence INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_logs_deploy ON deployment_logs(deployment_id, sequence);
CREATE INDEX IF NOT EXISTS idx_deployment_logs_correlation ON deployment_logs(correlation_id);
CREATE INDEX IF NOT EXISTS idx_deployment_logs_created ON deployment_logs(created_at DESC);

-- Beacon command logs with operation correlation
CREATE TABLE IF NOT EXISTS beacon_command_logs (
    id UUID PRIMARY KEY,
    command_id TEXT NOT NULL,
    operation_id TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    command_type TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    request_payload JSONB DEFAULT '{}',
    response_payload JSONB DEFAULT '{}',
    exit_code INT DEFAULT NULL,
    duration_ms BIGINT DEFAULT 0,
    error_message TEXT DEFAULT '',
    executed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_beacon_logs_command ON beacon_command_logs(command_id);
CREATE INDEX IF NOT EXISTS idx_beacon_logs_operation ON beacon_command_logs(operation_id);
CREATE INDEX IF NOT EXISTS idx_beacon_logs_correlation ON beacon_command_logs(correlation_id);
CREATE INDEX IF NOT EXISTS idx_beacon_logs_node ON beacon_command_logs(node_id, created_at DESC);

-- Structured correlation ID links
CREATE TABLE IF NOT EXISTS correlation_links (
    id UUID PRIMARY KEY,
    operation_id TEXT NOT NULL,
    command_id TEXT DEFAULT '',
    deployment_id TEXT DEFAULT '',
    build_id TEXT DEFAULT '',
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT DEFAULT '',
    parent_operation_id TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_corr_links_operation ON correlation_links(operation_id);
CREATE INDEX IF NOT EXISTS idx_corr_links_command ON correlation_links(command_id);
CREATE INDEX IF NOT EXISTS idx_corr_links_deployment ON correlation_links(deployment_id);

-- Alerts with severity and acknowledgement
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY,
    node_id UUID REFERENCES nodes(id) ON DELETE CASCADE,
    server_id UUID REFERENCES servers(id) ON DELETE CASCADE,
    alert_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('ok','warning','critical')),
    title TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    details JSONB DEFAULT '{}',
    source TEXT NOT NULL DEFAULT 'system',
    acknowledged BOOLEAN NOT NULL DEFAULT false,
    acknowledged_by TEXT DEFAULT '',
    acknowledged_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    suppression_key TEXT DEFAULT '',
    tenant_id TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alerts_node ON alerts(node_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_unack ON alerts(acknowledged, severity) WHERE acknowledged = false;
CREATE INDEX IF NOT EXISTS idx_alerts_suppression ON alerts(suppression_key);
CREATE INDEX IF NOT EXISTS idx_alerts_tenant ON alerts(tenant_id);

-- Notification routes (Slack, Discord, Telegram, Email, Webhook)
CREATE TABLE IF NOT EXISTS notification_routes (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('slack','discord','telegram','email','webhook')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    min_severity TEXT NOT NULL DEFAULT 'warning' CHECK (min_severity IN ('ok','warning','critical')),
    event_types TEXT[] DEFAULT '{}',
    tenant_id TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_routes_tenant ON notification_routes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_notification_routes_enabled ON notification_routes(enabled) WHERE enabled = true;

-- Health history per check type over time
CREATE TABLE IF NOT EXISTS health_history (
    id UUID PRIMARY KEY,
    check_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ok',
    message TEXT DEFAULT '',
    latency_ms BIGINT DEFAULT 0,
    details JSONB DEFAULT '{}',
    critical BOOLEAN NOT NULL DEFAULT false,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_health_history_check ON health_history(check_name, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_health_history_observed ON health_history(observed_at DESC);

-- Retention configuration for time-series data
CREATE TABLE IF NOT EXISTS retention_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_type TEXT NOT NULL UNIQUE,
    ttl_hours INT NOT NULL DEFAULT 168,
    max_records INT DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO retention_policies (metric_type, ttl_hours, max_records) VALUES
    ('node_metrics', 168, 100000),
    ('workload_metrics', 168, 100000),
    ('build_logs', 720, 50000),
    ('deployment_logs', 720, 50000),
    ('beacon_command_logs', 720, 50000),
    ('alerts', 720, 50000),
    ('health_history', 168, 50000),
    ('activity_events', 720, 100000)
ON CONFLICT (metric_type) DO NOTHING;
