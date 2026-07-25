-- 002_add_primary_allocation.sql (MySQL/MariaDB dialect)
-- Canonical declares primary_allocation_id as UUID (-> CHAR(36) here) with an
-- FK, plus a PARTIAL UNIQUE index:
--   CREATE UNIQUE INDEX servers_primary_allocation_id_unique
--       ON servers (primary_allocation_id) WHERE primary_allocation_id IS NOT NULL;
-- MySQL/MariaDB have no partial indexes. A plain UNIQUE index is the exact
-- equivalent here, because both engines treat NULL as distinct in a UNIQUE
-- index, so "unique among non-null values" is preserved. The index keeps the
-- canonical name so the two dialects converge on the same object.
ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS primary_allocation_id CHAR(36) NULL,
    ADD FOREIGN KEY (primary_allocation_id) REFERENCES allocations(id);

CREATE UNIQUE INDEX IF NOT EXISTS servers_primary_allocation_id_unique
    ON servers (primary_allocation_id);
