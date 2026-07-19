CREATE TABLE IF NOT EXISTS database_hosts (
    id UUID PRIMARY KEY,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    engine TEXT NOT NULL DEFAULT 'postgresql',
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 5432 CHECK (port BETWEEN 1 AND 65535),
    username TEXT NOT NULL,
    password TEXT NOT NULL DEFAULT '',
    max_databases INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE database_hosts
    ADD COLUMN IF NOT EXISTS engine TEXT NOT NULL DEFAULT 'postgresql';

CREATE TABLE IF NOT EXISTS server_databases (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    database_host_id UUID NOT NULL REFERENCES database_hosts(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    username TEXT NOT NULL,
    password TEXT NOT NULL,
    remote TEXT NOT NULL DEFAULT '%',
    max_connections INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (database_host_id, server_id, database_name),
    UNIQUE (database_host_id, username)
);

CREATE INDEX IF NOT EXISTS server_databases_server_id_idx ON server_databases (server_id);
CREATE INDEX IF NOT EXISTS database_hosts_node_id_idx ON database_hosts (node_id);

INSERT INTO database_hosts (id, node_id, engine, name, host, port, username, password, max_databases)
SELECT '99999999-9999-9999-9999-999999999999', n.id, 'postgresql', 'Local PostgreSQL', 'postgres', 5432, 'gamepanel', '', NULL
FROM nodes n
WHERE n.id = '22222222-2222-2222-2222-222222222222'
ON CONFLICT (id) DO NOTHING;

UPDATE database_hosts
SET engine = 'postgresql',
    name = 'Local PostgreSQL',
    host = 'postgres',
    port = 5432,
    username = 'gamepanel'
WHERE id = '99999999-9999-9999-9999-999999999999';
