ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS compose_path TEXT NOT NULL DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_source_id TEXT;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_webhook_id TEXT DEFAULT '';
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_next_poll_at TIMESTAMPTZ;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_auto_update BOOLEAN DEFAULT false;
ALTER TABLE compose_stacks ADD COLUMN IF NOT EXISTS git_update_status TEXT DEFAULT 'idle';

CREATE INDEX IF NOT EXISTS compose_stacks_git_source_idx ON compose_stacks(git_source_id) WHERE git_source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS compose_stacks_webhook_idx ON compose_stacks(git_webhook_id) WHERE git_webhook_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS compose_stacks_poll_idx ON compose_stacks(git_next_poll_at) WHERE git_auto_update = true;
CREATE INDEX IF NOT EXISTS compose_stacks_git_status_idx ON compose_stacks(git_update_status) WHERE git_update_status IN ('pending', 'deploying', 'rolling_back');
