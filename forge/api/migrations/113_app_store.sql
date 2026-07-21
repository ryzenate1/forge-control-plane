CREATE TABLE IF NOT EXISTS app_store_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    short_desc TEXT,
    description TEXT,
    icon VARCHAR(500),
    category VARCHAR(100),
    tags TEXT[],
    version VARCHAR(50) NOT NULL DEFAULT 'latest',
    compose_content TEXT NOT NULL,
    params JSONB,
    min_memory_mb INT,
    min_disk_mb INT,
    maintainer VARCHAR(255),
    source_url VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS app_store_installs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_key VARCHAR(100) NOT NULL,
    app_version VARCHAR(50) NOT NULL,
    project_id UUID,
    environment_id UUID,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'installing',
    params JSONB,
    compose_content TEXT NOT NULL,
    compose_project_id UUID,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
