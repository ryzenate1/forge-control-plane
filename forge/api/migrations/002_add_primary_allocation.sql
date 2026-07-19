ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS primary_allocation_id UUID REFERENCES allocations(id);

CREATE UNIQUE INDEX IF NOT EXISTS servers_primary_allocation_id_unique
    ON servers (primary_allocation_id)
    WHERE primary_allocation_id IS NOT NULL;
