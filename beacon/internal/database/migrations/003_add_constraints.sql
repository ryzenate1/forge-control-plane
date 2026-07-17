-- Add constraints for data integrity

-- Add NOT NULL constraint to servers.node_id
ALTER TABLE servers ADD CONSTRAINT fk_servers_node_id FOREIGN KEY (node_id) REFERENCES nodes(id);

-- Add NOT NULL constraint to backups.server_id
ALTER TABLE backups ADD CONSTRAINT fk_backups_server_id FOREIGN KEY (server_id) REFERENCES servers(id);

-- Add CHECK constraint for valid server status
ALTER TABLE servers ADD CONSTRAINT chk_servers_status CHECK (status IN ('starting', 'running', 'stopping', 'stopped', 'crashed'));

-- Add CHECK constraint for valid node status
ALTER TABLE nodes ADD CONSTRAINT chk_nodes_status CHECK (status IN ('online', 'offline', 'maintenance'));