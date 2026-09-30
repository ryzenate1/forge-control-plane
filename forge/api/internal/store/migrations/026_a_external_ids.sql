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
