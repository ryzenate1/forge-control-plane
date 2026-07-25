-- Phase 8 suite: supporting indexes and per-app targets produced by an
-- applied forge.yaml. forge_manifest_apps records the app/service/domain
-- links payload computed at apply time (URLs are placeholders until DNS is
-- configured for the base domain).
CREATE INDEX IF NOT EXISTS idx_forge_manifests_updated
    ON forge_manifests (updated_at DESC);

CREATE TABLE IF NOT EXISTS forge_manifest_apps (
    manifest_slug TEXT NOT NULL,
    app_id TEXT NOT NULL,
    app_name TEXT NOT NULL,
    service_id TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (manifest_slug, app_id)
);

CREATE INDEX IF NOT EXISTS idx_forge_manifest_apps_slug
    ON forge_manifest_apps (manifest_slug);