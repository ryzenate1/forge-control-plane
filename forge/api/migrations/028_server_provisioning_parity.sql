ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS docker_image TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS startup_command TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cpu_limit INTEGER NOT NULL DEFAULT 0 CHECK (cpu_limit >= 0);

CREATE INDEX IF NOT EXISTS servers_template_id_idx ON servers (template_id);
