-- Add scheduler backend support columns to nodes table

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS scheduler_type TEXT NOT NULL DEFAULT 'docker',
    ADD COLUMN IF NOT EXISTS scheduler_config JSONB;

-- Create index for scheduler type for filtering
CREATE INDEX IF NOT EXISTS nodes_scheduler_type_idx ON nodes (scheduler_type) WHERE scheduler_type IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN nodes.scheduler_type IS 'Scheduler backend used by this node (docker, k3s, nomad)';
COMMENT ON COLUMN nodes.scheduler_config IS 'JSON configuration for the scheduler backend (kubeconfig path, nomad addr, etc.)';
