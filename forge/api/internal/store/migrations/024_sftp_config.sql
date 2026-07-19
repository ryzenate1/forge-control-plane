-- SFTP per-node configuration
CREATE TABLE IF NOT EXISTS sftp_node_configs (
    node_id uuid PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT true,
    listen_port integer NOT NULL DEFAULT 2022,
    listen_ip text NOT NULL DEFAULT '0.0.0.0',
    max_connections integer NOT NULL DEFAULT 10,
    max_auth_attempts integer NOT NULL DEFAULT 3,
    idle_timeout integer NOT NULL DEFAULT 300,
    rate_limit integer NOT NULL DEFAULT 0,
    read_only boolean NOT NULL DEFAULT false,
    allowed_ips text[] DEFAULT '{}',
    banner text DEFAULT '',
    log_level text NOT NULL DEFAULT 'info',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- SFTP global configuration singleton
CREATE TABLE IF NOT EXISTS sftp_global_config (
    id integer PRIMARY KEY DEFAULT 1,
    enabled boolean NOT NULL DEFAULT true,
    default_port integer NOT NULL DEFAULT 2022,
    default_max_connections integer NOT NULL DEFAULT 10,
    default_idle_timeout integer NOT NULL DEFAULT 300,
    default_rate_limit integer NOT NULL DEFAULT 0,
    log_level text NOT NULL DEFAULT 'info',
    allowed_ciphers text[] DEFAULT '{}',
    allowed_macs text[] DEFAULT '{}',
    allowed_kex_algos text[] DEFAULT '{}',
    host_key_algorithms text[] DEFAULT '{}',
    CHECK (id = 1)
);
