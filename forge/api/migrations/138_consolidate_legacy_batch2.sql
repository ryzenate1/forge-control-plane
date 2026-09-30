-- Consolidated legacy Batch 2 schema.
--
-- Forge used to execute a second, allowlisted migration directory after the
-- canonical stream. That made the deployed schema depend on image copy lists
-- and allowed colliding version numbers in separate runners. This migration
-- absorbs the Batch 2 changes that were not already represented in the
-- canonical stream. Statements are intentionally idempotent so installations
-- previously upgraded through the old runner remain safe.

-- The omitted 024 Batch 2 migration created these SFTP list fields as TEXT,
-- whereas the canonical 084 migration uses TEXT[]. Normalise old installations
-- before the API scans them into []string. New installations already have the
-- canonical type and skip this block.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'sftp_node_configs'
          AND column_name = 'allowed_ips' AND data_type = 'text'
    ) THEN
        ALTER TABLE sftp_node_configs ALTER COLUMN allowed_ips TYPE TEXT[]
            USING CASE
                WHEN allowed_ips IS NULL OR btrim(allowed_ips) IN ('', '[]') THEN '{}'::TEXT[]
                ELSE string_to_array(regexp_replace(allowed_ips, '[\\[\\]"]', '', 'g'), ',')
            END;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'sftp_global_config'
          AND column_name = 'allowed_ciphers' AND data_type = 'text'
    ) THEN
        ALTER TABLE sftp_global_config ALTER COLUMN allowed_ciphers TYPE TEXT[]
            USING CASE WHEN allowed_ciphers IS NULL OR btrim(allowed_ciphers) IN ('', '[]') THEN '{}'::TEXT[] ELSE string_to_array(regexp_replace(allowed_ciphers, '[\\[\\]"]', '', 'g'), ',') END;
        ALTER TABLE sftp_global_config ALTER COLUMN allowed_macs TYPE TEXT[]
            USING CASE WHEN allowed_macs IS NULL OR btrim(allowed_macs) IN ('', '[]') THEN '{}'::TEXT[] ELSE string_to_array(regexp_replace(allowed_macs, '[\\[\\]"]', '', 'g'), ',') END;
        ALTER TABLE sftp_global_config ALTER COLUMN allowed_kex_algos TYPE TEXT[]
            USING CASE WHEN allowed_kex_algos IS NULL OR btrim(allowed_kex_algos) IN ('', '[]') THEN '{}'::TEXT[] ELSE string_to_array(regexp_replace(allowed_kex_algos, '[\\[\\]"]', '', 'g'), ',') END;
        ALTER TABLE sftp_global_config ALTER COLUMN host_key_algorithms TYPE TEXT[]
            USING CASE WHEN host_key_algorithms IS NULL OR btrim(host_key_algorithms) IN ('', '[]') THEN '{}'::TEXT[] ELSE string_to_array(regexp_replace(host_key_algorithms, '[\\[\\]"]', '', 'g'), ',') END;
    END IF;
END $$;

-- Former Batch 2: 025_a_install_workflows.sql
CREATE TABLE IF NOT EXISTS install_workflows (
    id uuid PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    type text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    steps jsonb DEFAULT '[]',
    metadata jsonb DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_install_workflows_server ON install_workflows(server_id);

-- Former Batch 2: 026_a_external_ids.sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_users_external_id ON users (external_id) WHERE external_id != '';

ALTER TABLE servers ADD COLUMN IF NOT EXISTS external_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_servers_external_id ON servers (external_id) WHERE external_id != '';

CREATE TABLE IF NOT EXISTS panel_maintenance_settings (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT only_one_row CHECK (id)
);

-- Former Batch 2: 033_node_onboarding.sql
-- Node enrollment and capability synchronization
-- Inspired by Komodo Core/Periphery onboarding key patterns

CREATE TABLE IF NOT EXISTS onboarding_tokens (
    id UUID PRIMARY KEY,
    token_hash TEXT NOT NULL,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    approved_by TEXT,
    revoked_at TIMESTAMPTZ,
    revoked_reason TEXT,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'revoked', 'expired')),
    UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS onboarding_tokens_node_id_idx ON onboarding_tokens (node_id);
CREATE INDEX IF NOT EXISTS onboarding_tokens_state_idx ON onboarding_tokens (state);

CREATE TABLE IF NOT EXISTS node_capabilities (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    beacon_version TEXT NOT NULL DEFAULT '',
    os TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    cpu_threads INTEGER NOT NULL DEFAULT 0,
    memory_mb BIGINT NOT NULL DEFAULT 0,
    disk_mb BIGINT NOT NULL DEFAULT 0,
    uptime_seconds BIGINT NOT NULL DEFAULT 0,

    -- Capability flags
    runtime_available BOOLEAN NOT NULL DEFAULT FALSE,
    runtime_status TEXT NOT NULL DEFAULT '',
    runtime_version TEXT NOT NULL DEFAULT '',
    runtime_provider TEXT NOT NULL DEFAULT '',

    docker_build_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    nixpacks_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    compose_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    compose_version TEXT NOT NULL DEFAULT '',
    compose_stack_count INTEGER NOT NULL DEFAULT 0,

    local_backups BOOLEAN NOT NULL DEFAULT FALSE,
    s3_backups BOOLEAN NOT NULL DEFAULT FALSE,
    transfer_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    sftp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    websocket_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    console_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    database_provisioning_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    -- Full JSON capability report for extensibility
    raw_report JSONB NOT NULL DEFAULT '{}',

    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (node_id)
);

CREATE INDEX IF NOT EXISTS node_capabilities_fetched_at_idx ON node_capabilities (fetched_at DESC);

-- Node capability history for tracking changes over time
CREATE TABLE IF NOT EXISTS node_capability_history (
    id UUID PRIMARY KEY,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    beacon_version TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]',
    raw_report JSONB NOT NULL DEFAULT '{}',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS node_capability_history_node_id_idx ON node_capability_history (node_id, observed_at DESC);

-- Former Batch 2: 034_build_pipeline.sql
ALTER TABLE builds ADD COLUMN IF NOT EXISTS node_id TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS registry TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS cache_from TEXT[] DEFAULT '{}';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS cache_to TEXT[] DEFAULT '{}';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS platform TEXT DEFAULT 'linux/amd64';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS commit_sha TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS commit_ref TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS digest TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS retry_of TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS retry_attempt INTEGER DEFAULT 0;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS timed_out BOOLEAN DEFAULT FALSE;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS build_timeout_secs INTEGER DEFAULT 1800;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS credentials_masked BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS builds_node_idx ON builds (node_id);
CREATE INDEX IF NOT EXISTS builds_status_idx ON builds (status);

-- Former Batch 2: 035_compose_gitops.sql
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_source_id TEXT;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_repository_url TEXT;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_repository_path TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_branch TEXT DEFAULT 'main';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_commit_sha TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_desired_commit_sha TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_previous_commit_sha TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_previous_compose_yaml TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_auto_update BOOLEAN DEFAULT false;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_poll_interval_seconds INTEGER DEFAULT 300;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_webhook_secret TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_webhook_id TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_last_webhook_at TIMESTAMPTZ;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_status TEXT DEFAULT 'idle';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_error TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_next_poll_at TIMESTAMPTZ;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_credential_id TEXT;

-- Former Batch 2: 035_a_infra_endpoints.sql
CREATE TABLE IF NOT EXISTS infra_endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    endpoint_type TEXT NOT NULL DEFAULT 'docker',
    connection_mode TEXT NOT NULL DEFAULT 'direct',
    status TEXT NOT NULL DEFAULT 'unknown',
    edge_id TEXT,
    tls_ca TEXT,
    tls_cert TEXT,
    tls_key TEXT,
    tags TEXT[] DEFAULT '{}',
    labels JSONB DEFAULT '[]',
    url TEXT DEFAULT '',
    project_id TEXT,
    group_id TEXT,
    reachable BOOLEAN DEFAULT false,
    version TEXT DEFAULT '',
    total_container_count INTEGER DEFAULT 0,
    total_image_count INTEGER DEFAULT 0,
    total_volume_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS infra_endpoint_nodes (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(endpoint_id, node_id)
);

CREATE TABLE IF NOT EXISTS infra_endpoint_access_policies (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(endpoint_id, principal_type, principal_id)
);

CREATE TABLE IF NOT EXISTS infra_endpoint_health_history (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES infra_endpoints(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    reachable BOOLEAN NOT NULL DEFAULT false,
    health_score REAL DEFAULT 0,
    version TEXT DEFAULT '',
    total_containers INTEGER DEFAULT 0,
    total_images INTEGER DEFAULT 0,
    total_volumes INTEGER DEFAULT 0,
    error_message TEXT DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_endpoint_nodes_endpoint ON infra_endpoint_nodes(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_nodes_node ON infra_endpoint_nodes(node_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_policies_endpoint ON infra_endpoint_access_policies(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_health_endpoint ON infra_endpoint_health_history(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_health_observed ON infra_endpoint_health_history(endpoint_id, observed_at DESC);

-- Former Batch 2: 035_b_observability_monitoring.sql
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
CREATE INDEX IF NOT EXISTS idx_alerts_server ON alerts(server_id, created_at DESC);
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

-- Former Batch 2: 036_compose_concurrency.sql
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_claimed_by TEXT;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_claimed_at TIMESTAMPTZ;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_last_delivery_id TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_reconcile_mode TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_failed_sha TEXT;

-- Former Batch 2: 037_build_extended_fields.sql
ALTER TABLE builds ADD COLUMN IF NOT EXISTS build_stage TEXT DEFAULT 'queued';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS workspace_id TEXT DEFAULT '';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS idempotency_key TEXT DEFAULT '';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS beacon_build_id TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS builds_beacon_build_idx ON builds (beacon_build_id);

-- Former Batch 2: 041_buildpack_support.sql
CREATE TABLE IF NOT EXISTS buildpacks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    builder_type TEXT NOT NULL DEFAULT 'herokuish'
        CHECK (builder_type IN ('herokuish','cnb','nixpacks','railpack')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS buildpacks_type_idx ON buildpacks (builder_type);

CREATE TABLE IF NOT EXISTS server_buildpacks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id    UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    buildpack_id UUID NOT NULL REFERENCES buildpacks(id) ON DELETE CASCADE,
    priority     INT NOT NULL DEFAULT 0,
    UNIQUE (server_id, buildpack_id)
);

CREATE INDEX IF NOT EXISTS server_buildpacks_server_idx ON server_buildpacks (server_id);

CREATE TABLE IF NOT EXISTS app_builds (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id    UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    buildpack_id UUID REFERENCES buildpacks(id) ON DELETE SET NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','running','succeeded','failed','canceled')),
    build_log    TEXT NOT NULL DEFAULT '',
    image_tag    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS app_builds_server_idx ON app_builds (server_id);
CREATE INDEX IF NOT EXISTS app_builds_status_idx ON app_builds (status);

-- Former Batch 2: 042_service_discovery_endpoints.sql
CREATE TABLE IF NOT EXISTS service_discovery_endpoints (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL,
    service_id      TEXT NOT NULL DEFAULT '',
    node_id         TEXT NOT NULL,
    node_name       TEXT NOT NULL DEFAULT '',
    region_id       TEXT,
    address         TEXT NOT NULL,
    port            INTEGER NOT NULL,
    protocol        TEXT NOT NULL DEFAULT 'tcp',
    status          TEXT NOT NULL DEFAULT 'unknown',
    replica_index   INTEGER NOT NULL DEFAULT 0,
    tenant_id       TEXT,
    last_heartbeat  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_svc_disc_svc_name ON service_discovery_endpoints (service_name);
CREATE INDEX IF NOT EXISTS idx_svc_disc_node_id  ON service_discovery_endpoints (node_id);
CREATE INDEX IF NOT EXISTS idx_svc_disc_tenant_id ON service_discovery_endpoints (tenant_id);

-- Former Batch 2: 043_webhook_idempotency.sql
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS idempotency_key TEXT DEFAULT '';
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS idempotency_key_processed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_idempotency_key ON webhook_deliveries(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key != '';
