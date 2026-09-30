-- traffic_rules table is defined in migration 083_a_traffic_rules.sql
-- This migration adds the web_socket column if missing and FK constraints.

ALTER TABLE traffic_rules ADD COLUMN IF NOT EXISTS web_socket BOOLEAN NOT NULL DEFAULT FALSE;

-- The retired bare-083 copy (083_traffic_rules.sql) created server_id as TEXT
-- with a '' default, while the canonical 083_a defines it as UUID. The legacy
-- copy sorts first, so on a fresh install the table already exists (as TEXT)
-- when the canonical CREATE TABLE IF NOT EXISTS becomes a no-op. Converge the
-- column to the canonical type before adding the FK, otherwise the FK fails
-- with "foreign key constraint cannot be implemented" (SQLSTATE 42804).
-- Hosts that already carry UUID skip the body via the information_schema
-- guard. Rows still holding the legacy '' default cannot cast to UUID and
-- fail loudly here instead of silently skipping the FK.
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'traffic_rules'
          AND column_name = 'server_id' AND data_type = 'text'
    ) THEN
        ALTER TABLE traffic_rules ALTER COLUMN server_id DROP DEFAULT;
        ALTER TABLE traffic_rules ALTER COLUMN server_id TYPE UUID USING NULLIF(server_id, '')::uuid;
    END IF;
END $$;

DO $$ BEGIN ALTER TABLE traffic_rules ADD CONSTRAINT fk_traffic_rules_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);
