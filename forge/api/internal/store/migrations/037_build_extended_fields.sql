ALTER TABLE builds ADD COLUMN IF NOT EXISTS build_stage TEXT DEFAULT 'queued';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS workspace_id TEXT DEFAULT '';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS idempotency_key TEXT DEFAULT '';
ALTER TABLE builds ADD COLUMN IF NOT EXISTS beacon_build_id TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS builds_beacon_build_idx ON builds (beacon_build_id);
