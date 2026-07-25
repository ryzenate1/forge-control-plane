-- Backup engines: Restic / Kopia repository management alongside the
-- existing dbprovisioner/dbbackupsvc paths. Operators register a repository
-- (S3 / local / rest: / sftp: / gcrypt: ...), snapshot it on the existing
-- cron schedule, and browse/verify/restore/prune snapshots from the admin UI.
-- CLI execution is routed through the beacon daemon on the bound node/server.

CREATE TABLE IF NOT EXISTS backup_repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    engine VARCHAR(20) NOT NULL CHECK (engine IN ('restic', 'kopia')),
    location VARCHAR(2048) NOT NULL,
    -- Repository password. `password_ref` holds the plaintext operator input
    -- (or an "env:VAR" name resolved on the node); `password_encrypted` is
    -- the sealed envelope written by the store secret layer at rest.
    password_ref TEXT NOT NULL DEFAULT '',
    password_encrypted TEXT NOT NULL DEFAULT '',
    encryption VARCHAR(50) NOT NULL DEFAULT 'aes-256',
    -- Prune/retention policy + snapshot cron schedule, JSONB:
    -- { "schedule": "0 3 * * *", "keep_last": 10, "keep_daily": 14, ... }
    prune_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Execution routing: exactly which beacon node runs the CLI. server_id
    -- is preferred when set (the node is resolved from the server binding).
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    -- Optional artifact lineage: snapshots taken for a Forge server can be
    -- correlated with backup_artifacts rows produced by the classic pipeline.
    artifact_id UUID REFERENCES backup_artifacts(id) ON DELETE SET NULL,
    initialized BOOLEAN NOT NULL DEFAULT FALSE,
    last_snapshot_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT backup_repositories_unique_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_backup_repositories_engine ON backup_repositories(engine);
CREATE INDEX IF NOT EXISTS idx_backup_repositories_node ON backup_repositories(node_id);
CREATE INDEX IF NOT EXISTS idx_backup_repositories_next_run ON backup_repositories(next_run_at);

CREATE TABLE IF NOT EXISTS backup_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES backup_repositories(id) ON DELETE CASCADE,
    -- Snapshot short id as reported by `restic snapshots --json` /
    -- `kopia snapshot list --json` (hex-free: displayed as an opaque id).
    snapshot_id VARCHAR(128) NOT NULL,
    paths JSONB NOT NULL DEFAULT '[]'::jsonb,
    hostname VARCHAR(255) NOT NULL DEFAULT '',
    snapshot_time TIMESTAMPTZ,
    parent VARCHAR(128),
    tree VARCHAR(128),
    data_files BIGINT NOT NULL DEFAULT 0,
    total_size_bytes BIGINT NOT NULL DEFAULT 0,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT backup_snapshots_repo_unique UNIQUE (repo_id, snapshot_id)
);

CREATE INDEX IF NOT EXISTS idx_backup_snapshots_repo_time ON backup_snapshots(repo_id, snapshot_time DESC);

CREATE TABLE IF NOT EXISTS backup_restore_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id UUID NOT NULL REFERENCES backup_repositories(id) ON DELETE CASCADE,
    snapshot_id VARCHAR(128) NOT NULL,
    target_path VARCHAR(1024) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    progress_pct INT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_repo ON backup_restore_jobs(repo_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_status ON backup_restore_jobs(status);
