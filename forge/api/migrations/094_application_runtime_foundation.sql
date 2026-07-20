-- Make one application revision placement idempotent. A retry resumes the
-- same instance instead of creating parallel containers on the same Beacon.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_instances_revision_node
    ON workload_instances(workload_id, revision_id, node_id)
    WHERE node_id IS NOT NULL;

-- The durable queue predates canonical workloads and historically assumed
-- every resource was a game server. Retain server_id for compatibility while
-- making the resource kind explicit for new capability modules.
ALTER TABLE job_queue
    ADD COLUMN IF NOT EXISTS resource_type text NOT NULL DEFAULT 'server';

CREATE INDEX IF NOT EXISTS idx_job_queue_resource
    ON job_queue(resource_type, server_id, created_at DESC);
