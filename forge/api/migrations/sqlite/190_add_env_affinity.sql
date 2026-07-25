-- SQLite dialect override for 190_add_env_affinity.sql
-- Phase 6: env-aware placement affinity.
-- Servers declare the environment (org/project/env group) they belong to and
-- nodes advertise the environment groups they serve. The env-affinity engine
-- (internal/services/envaffinity) reads these columns to populate
-- ConstraintContext.NodeLabels and pin label affinity during placement.
ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS env_affinity TEXT;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS env_groups TEXT[] DEFAULT '{}';

CREATE INDEX IF NOT EXISTS servers_env_affinity_idx ON servers (env_affinity);
-- SQLite encoding: no GIN here; containment scans fall back to full scan
-- on sqlite dev/test databases. Production PostgreSQL keeps the GIN index.