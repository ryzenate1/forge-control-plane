-- Add generation and workload_lease_expiry to servers
ALTER TABLE servers ADD COLUMN IF NOT EXISTS generation bigint NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS workload_lease_expiry timestamptz;

-- Add fence_generation to recovery_items
ALTER TABLE recovery_items ADD COLUMN IF NOT EXISTS fence_generation bigint NOT NULL DEFAULT 0;

-- Add new enum values
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'awaiting_beacon';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'health_gating';
