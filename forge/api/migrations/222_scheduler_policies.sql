-- 222: persist scheduler affinity/anti-affinity rules and node constraints
-- so they survive restarts instead of being in-memory-only.
CREATE TABLE IF NOT EXISTS server_affinity_rules (
    id          TEXT PRIMARY KEY,
    node_id     UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_tag  TEXT NOT NULL DEFAULT '',
    weight      REAL NOT NULL DEFAULT 1.0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS server_anti_affinity_rules (
    id          TEXT PRIMARY KEY,
    node_id     UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    target_tag  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS server_constraints (
    id          TEXT PRIMARY KEY,
    node_id     UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL DEFAULT '',
    operator    TEXT NOT NULL DEFAULT 'equal',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(node_id, key)
);
