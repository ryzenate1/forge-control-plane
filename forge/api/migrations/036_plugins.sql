-- Plugins (Pelican parity). Pelican stores a JSON manifest describing each
-- plugin in the database and lets admins import from a file or URL, install
-- (run setup script), enable/disable (toggle loading), and uninstall. We
-- store the manifest inline in the row so plugins can be queried quickly.

CREATE TABLE IF NOT EXISTS plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    -- "theme", "integration", "tool", "auth", etc.
    kind TEXT NOT NULL DEFAULT 'integration',
    version TEXT NOT NULL DEFAULT '0.0.0',
    -- JSON: arbitrary manifest payload (config defaults, hooks, files).
    manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- The path on disk where the plugin files live after install.
    install_path TEXT,
    -- Boolean state: has install() been run?
    installed BOOLEAN NOT NULL DEFAULT FALSE,
    -- Boolean state: is the plugin currently active?
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    -- Where did this plugin come from? "file" or "url:<url>".
    source TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS plugins_kind_idx ON plugins (kind);
CREATE INDEX IF NOT EXISTS plugins_enabled_idx ON plugins (enabled);