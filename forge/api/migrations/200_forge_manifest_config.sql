-- Phase 8 (env-as-code): declarative forge.yaml manifests.
-- One row per project (natural key = project slug), so re-applying a newer
-- forge.yaml idempotently replaces the previous manifest for the same project.
CREATE TABLE IF NOT EXISTS forge_manifests (
    project_slug TEXT PRIMARY KEY,
    project_name TEXT NOT NULL DEFAULT '',
    content JSONB NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    applied_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);