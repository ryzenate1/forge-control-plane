-- 229_a: Real-time Docker events feed.
--
-- Every Beacon node tails its local Docker event stream (container lifecycle:
-- start, stop, die, kill, oom, recreate, destroy) and batches what it sees into
-- POST /api/remote/docker/events. The panel stores the raw fact here so the
-- admin console can render a cluster-wide activity timeline and the
-- notifications engine can fan `docker.event.<type>` out to subscribed
-- channels.
--
-- Named 229_a rather than the planned 229 because the bare 229 slot belongs to
-- 229_server_runtime_provider.sql. migrationPrefix() in
-- internal/store/migration.go treats a letter suffix as part of the prefix, so
-- "229_a" and "229" are distinct and both validate — the established pattern in
-- this directory (024_a/024, 225_a/225, 226_a/226, 228_a/228). Renaming the
-- already-numbered file instead would have been worse than it sounds:
-- schema_migrations.version stores the full filename, so a shipped migration
-- that changes name is re-applied as if it were brand new.
CREATE TABLE IF NOT EXISTS docker_events (
    id             BIGSERIAL PRIMARY KEY,
    node_id        UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    event_type     VARCHAR(32) NOT NULL,
    container_id   VARCHAR(64) NOT NULL DEFAULT '',
    container_name TEXT NOT NULL DEFAULT '',
    image          TEXT NOT NULL DEFAULT '',
    actor_attrs    JSONB NOT NULL DEFAULT '{}'::jsonb,
    "timestamp"    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ingested_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Timeline scan (newest first) and the per-node variant the feed's node filter
-- uses; event_type backs the type filter and the notification audit lookup.
CREATE INDEX IF NOT EXISTS idx_docker_events_ts ON docker_events ("timestamp" DESC);
CREATE INDEX IF NOT EXISTS idx_docker_events_node_ts ON docker_events (node_id, "timestamp" DESC);
CREATE INDEX IF NOT EXISTS idx_docker_events_type ON docker_events (event_type);

-- A Beacon reconnect replays whatever was still in its local buffer, so the
-- same (node, container, action, docker-timestamp) fact can arrive twice. The
-- ingest insert relies on this with ON CONFLICT DO NOTHING, which makes
-- delivery at-least-once without duplicating the timeline.
CREATE UNIQUE INDEX IF NOT EXISTS idx_docker_events_dedupe
    ON docker_events (node_id, container_id, event_type, "timestamp");
