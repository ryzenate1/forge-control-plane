ALTER TABLE allocations
    ADD COLUMN IF NOT EXISTS protocol TEXT NOT NULL DEFAULT 'tcp',
    ADD COLUMN IF NOT EXISTS container_port INTEGER;

UPDATE allocations SET container_port = port WHERE container_port IS NULL;

ALTER TABLE allocations
    ALTER COLUMN container_port SET NOT NULL,
    DROP CONSTRAINT IF EXISTS allocations_node_id_ip_port_key;

ALTER TABLE allocations
    ADD CONSTRAINT allocations_protocol_check CHECK (protocol IN ('tcp', 'udp')),
    ADD CONSTRAINT allocations_container_port_check CHECK (container_port BETWEEN 1 AND 65535),
    ADD CONSTRAINT allocations_node_ip_port_protocol_key UNIQUE (node_id, ip, port, protocol);
