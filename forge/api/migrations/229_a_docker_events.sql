-- 229_a: Real-time Docker events feed.
--
-- Every Beacon node tails its local Docker event stream (container lifecycle:
-- start, stop, die, kill, oom, recreate, destroy) and batches what it sees into
-- POST /api/remote/docker/events. The panel stores the raw fact here so the
-- admin console can render a cluster-wide activity timeline and the
-- notifications engine can fan `docker.event.<type>` out to subscribed
-- channels.
--
-- Named 229_a (not 229 as planned) because 229_server_runtime_provider.sql
-- already occupies the bare 229 prefix and the duplicate-prefix validator in
-- internal/store/migration.go rejects a second file with the same numeric
-- prefix; the letter-suffix form is the repo's established escape hatch
-- (225_a, 226_a, 211_a/211_b).
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
