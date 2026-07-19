-- Add runtime_status column to nodes table for tracking runtime provider health

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS runtime_provider TEXT,
    ADD COLUMN IF NOT EXISTS runtime_status TEXT;

-- Create index for runtime provider and status for filtering
CREATE INDEX IF NOT EXISTS nodes_runtime_provider_idx ON nodes (runtime_provider) WHERE runtime_provider IS NOT NULL;
CREATE INDEX IF NOT EXISTS nodes_runtime_status_idx ON nodes (runtime_status) WHERE runtime_status IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN nodes.runtime_status IS 'Current status of the runtime provider (ok, error, degraded)';
COMMENT ON COLUMN nodes.runtime_provider IS 'Runtime provider used by this node (docker, kubernetes, podman, etc.)';