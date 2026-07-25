-- Phase 2: environment manifest (service-definition) engine.
--
-- Stores the last applied EnvManifest YAML/JSON per environment so the
-- engine can diff, re-apply and render it back through the API
-- (GET/PUT /api/v1/envs/:id/manifest).
CREATE TABLE IF NOT EXISTS env_manifests (
    env_id       UUID PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
    version      TEXT NOT NULL DEFAULT '1',
    manifest     JSONB NOT NULL DEFAULT '{}'::jsonb,
    manifest_yaml TEXT NOT NULL DEFAULT '',
    applied_by   UUID REFERENCES users(id),
    applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_env_manifests_applied_at ON env_manifests (applied_at DESC);
