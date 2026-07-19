ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS primary_allocation_id CHAR(36) NULL,
    ADD FOREIGN KEY (primary_allocation_id) REFERENCES allocations(id);

CREATE INDEX IF NOT EXISTS servers_primary_allocation_id_idx
    ON servers (primary_allocation_id);
