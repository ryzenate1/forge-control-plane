ALTER TABLE servers ADD COLUMN primary_allocation_id TEXT REFERENCES allocations(id);

CREATE INDEX IF NOT EXISTS servers_primary_allocation_id_idx
    ON servers (primary_allocation_id);
