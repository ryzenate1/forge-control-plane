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
CREATE INDEX IF NOT EXISTS nodes_env_groups_idx ON nodes USING GIN (env_groups);