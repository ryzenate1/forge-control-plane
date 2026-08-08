CREATE TABLE IF NOT EXISTS buildpacks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL DEFAULT '',
    builder_type TEXT NOT NULL DEFAULT 'herokuish'
        CHECK (builder_type IN ('herokuish','cnb','nixpacks','railpack')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS buildpacks_type_idx ON buildpacks (builder_type);

CREATE TABLE IF NOT EXISTS server_buildpacks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id    UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    buildpack_id UUID NOT NULL REFERENCES buildpacks(id) ON DELETE CASCADE,
    priority     INT NOT NULL DEFAULT 0,
    UNIQUE (server_id, buildpack_id)
);

CREATE INDEX IF NOT EXISTS server_buildpacks_server_idx ON server_buildpacks (server_id);

CREATE TABLE IF NOT EXISTS app_builds (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id    UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    buildpack_id UUID REFERENCES buildpacks(id) ON DELETE SET NULL,
    status       TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','running','succeeded','failed','canceled')),
    build_log    TEXT NOT NULL DEFAULT '',
    image_tag    TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS app_builds_server_idx ON app_builds (server_id);
CREATE INDEX IF NOT EXISTS app_builds_status_idx ON app_builds (status);
