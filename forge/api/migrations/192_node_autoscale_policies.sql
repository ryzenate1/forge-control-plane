-- Phase 6: node autoscaling policies. A policy scales an entire cluster group
-- (or the whole fleet when cluster_group_id is null) by provisioning cloud
-- instances through cloud.Manager and joining them into the cluster.
CREATE TABLE IF NOT EXISTS node_autoscale_policies (
    id                UUID PRIMARY KEY,
    name              TEXT NOT NULL,
    cluster_group_id  TEXT,
    provider          TEXT NOT NULL DEFAULT 'aws',
    region            TEXT NOT NULL DEFAULT '',
    instance_type     TEXT NOT NULL DEFAULT '',
    image             TEXT NOT NULL DEFAULT '',
    min_nodes         INTEGER NOT NULL DEFAULT 1,
    max_nodes         INTEGER NOT NULL DEFAULT 8,
    target_cpu_percent REAL NOT NULL DEFAULT 80,
    target_mem_percent REAL NOT NULL DEFAULT 80,
    evaluator         TEXT NOT NULL DEFAULT 'cpu',
    auto_join         BOOLEAN NOT NULL DEFAULT false,
    cooldown_seconds  INTEGER NOT NULL DEFAULT 900,
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);