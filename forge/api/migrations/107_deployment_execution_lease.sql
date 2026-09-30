ALTER TABLE deployments ADD COLUMN IF NOT EXISTS executor_id TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS execution_lease_until TIMESTAMPTZ;
