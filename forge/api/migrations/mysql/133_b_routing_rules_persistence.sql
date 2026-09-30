-- 133_b_routing_rules_persistence.sql (MySQL/MariaDB dialect)
--
-- The traffic_rules table is created by mysql/083_a_traffic_rules.sql. Canonical
-- 133_b is the FK FOLLOWUP for that table -- it is the value tracked for
-- traffic_rules in fkFollowups / fkFollowupMigrations
-- (internal/store/migration.go) -- and it adds web_socket if a host only has the
-- bare 083 copy.
--
-- The previous override here re-declared the whole CREATE TABLE instead of the
-- followup work, so (a) on MySQL hosts traffic_rules never received
-- fk_traffic_rules_server, leaving server_id unenforced and orphan-prone, and
-- (b) two divergent definitions of the same table existed in the dialect dir.
-- This file now performs exactly the canonical job.
ALTER TABLE traffic_rules ADD COLUMN IF NOT EXISTS web_socket BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE traffic_rules ADD CONSTRAINT IF NOT EXISTS fk_traffic_rules_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS traffic_rules_server_id_idx ON traffic_rules (server_id);
CREATE INDEX IF NOT EXISTS traffic_rules_enabled_idx ON traffic_rules (enabled);
