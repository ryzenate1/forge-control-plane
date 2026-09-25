-- 225_tags_and_mounts.sql
-- First-class color tags (assignable to applications, servers, environments) plus
-- declarative per-application persistent mounts (Dokploy/Dokku inspired).
--
-- Tags are a global catalog (shared vocabulary across the panel); resource_tags is
-- the many-to-many assignment join table. app_mounts stores per-application
-- persistent storage that survives redeploys: named volumes, host bind mounts,
-- tmpfs, or DB-stored seed files materialized at deploy time.

-- ---- Tags -----------------------------------------------------------------

CREATE TABLE IF NOT EXISTS tags (
    id          UUID PRIMARY KEY,
    name        VARCHAR NOT NULL UNIQUE,
    color       VARCHAR(7) NOT NULL DEFAULT '#6366f1' CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tags_name ON tags (name);

CREATE TABLE IF NOT EXISTS resource_tags (
    id            UUID PRIMARY KEY,
    tag_id        UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    resource_type VARCHAR NOT NULL CHECK (resource_type IN ('application', 'server', 'environment')),
    resource_id   UUID NOT NULL,
    assigned_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tag_id, resource_type, resource_id)
);

-- Look up tags for a single resource (detail views) and the inverse (filtering a
-- list by tag, and bulk operations that resolve "everything tagged X").
CREATE INDEX IF NOT EXISTS idx_resource_tags_resource ON resource_tags (resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_tags_tag ON resource_tags (tag_id);

-- ---- Per-application persistent mounts ------------------------------------

CREATE TABLE IF NOT EXISTS app_mounts (
    id             UUID PRIMARY KEY,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    type           VARCHAR NOT NULL CHECK (type IN ('volume', 'bind', 'tmpfs', 'seed-file')),
    source         TEXT NOT NULL DEFAULT '',
    target         TEXT NOT NULL,
    read_only      BOOLEAN NOT NULL DEFAULT false,
    content        TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_mounts_application_id ON app_mounts (application_id);
-- A container path is only meaningful once per app; reject duplicate targets so
-- compose injection never produces colliding mounts.
CREATE UNIQUE INDEX IF NOT EXISTS uq_app_mounts_app_target ON app_mounts (application_id, target);
