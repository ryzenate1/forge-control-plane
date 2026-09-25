-- Upgrade plan records for the control-plane self-upgrade system.
-- Each row is one planned or executed upgrade of a component (api, web, beacon,
-- database) or a full-stack upgrade. The service (internal/services/upgrade)
-- reads/writes this table; handlers_upgrade.go exposes it over HTTP.
CREATE TABLE IF NOT EXISTS upgrade_plans (
    id            TEXT PRIMARY KEY,
    type          TEXT NOT NULL DEFAULT 'full',
    from_version  TEXT NOT NULL DEFAULT '',
    to_version    TEXT NOT NULL DEFAULT '',
    components    JSONB NOT NULL DEFAULT '[]',
    status        TEXT NOT NULL DEFAULT 'pending',
    progress      INTEGER NOT NULL DEFAULT 0,
    total_steps   INTEGER NOT NULL DEFAULT 0,
    current_step  TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    backup_path   TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_upgrade_plans_status ON upgrade_plans (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_upgrade_plans_created ON upgrade_plans (created_at DESC);
