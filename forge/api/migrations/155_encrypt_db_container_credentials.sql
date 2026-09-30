ALTER TABLE db_containers
    ADD COLUMN IF NOT EXISTS connection_string_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '';
