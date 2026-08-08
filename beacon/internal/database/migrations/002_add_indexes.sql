-- Add indexes for performance optimization

-- Index for server name
CREATE INDEX IF NOT EXISTS idx_servers_name ON servers(name);

-- Index for server status
CREATE INDEX IF NOT EXISTS idx_servers_status ON servers(status);

-- Index for node name
CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);

-- Index for node status
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);

-- Index for backup server_id
CREATE INDEX IF NOT EXISTS idx_backups_server_id ON backups(server_id);