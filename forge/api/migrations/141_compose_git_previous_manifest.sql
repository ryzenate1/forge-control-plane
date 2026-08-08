ALTER TABLE compose_stacks
    ADD COLUMN IF NOT EXISTS git_previous_manifest JSONB,
    ADD COLUMN IF NOT EXISTS parsed_config JSONB NOT NULL DEFAULT '{}';
