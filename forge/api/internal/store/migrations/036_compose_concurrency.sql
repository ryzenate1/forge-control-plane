ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_claimed_by TEXT;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_claimed_at TIMESTAMPTZ;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_last_delivery_id TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_reconcile_mode TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_failed_sha TEXT;
