-- traffic_rules table is defined in migration 083_a_traffic_rules.sql
-- This migration adds the web_socket column if missing and FK constraints.

ALTER TABLE traffic_rules ADD COLUMN IF NOT EXISTS web_socket BOOLEAN NOT NULL DEFAULT FALSE;
DO $$ BEGIN ALTER TABLE traffic_rules ADD CONSTRAINT fk_traffic_rules_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);
