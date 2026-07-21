CREATE TABLE IF NOT EXISTS deployment_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    version INT NOT NULL,
    image_tag TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','building','deploying','health_checking','live','rolled_back','failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deployment_releases_server
    ON deployment_releases(server_id, version DESC);

CREATE TABLE IF NOT EXISTS health_check_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    path TEXT NOT NULL DEFAULT '/health',
    port INT NOT NULL DEFAULT 80,
    protocol TEXT NOT NULL DEFAULT 'http',
    interval_seconds INT NOT NULL DEFAULT 10,
    timeout_seconds INT NOT NULL DEFAULT 5,
    healthy_threshold INT NOT NULL DEFAULT 2,
    unhealthy_threshold INT NOT NULL DEFAULT 3,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(server_id)
);

CREATE TABLE IF NOT EXISTS health_check_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployment_releases(id) ON DELETE CASCADE,
    check_timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    status TEXT NOT NULL CHECK (status IN ('healthy','unhealthy','error')),
    response_code INT NOT NULL DEFAULT 0,
    response_time_ms INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_health_check_results_deployment
    ON health_check_results(deployment_id, check_timestamp DESC);

CREATE TABLE IF NOT EXISTS deployment_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id UUID NOT NULL REFERENCES deployment_releases(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_events_deployment
    ON deployment_events(deployment_id, created_at ASC);
