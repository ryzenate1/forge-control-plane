ALTER TABLE compose_stacks
    ADD COLUMN IF NOT EXISTS env_vars_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS git_webhook_secret_encrypted TEXT NOT NULL DEFAULT '';
