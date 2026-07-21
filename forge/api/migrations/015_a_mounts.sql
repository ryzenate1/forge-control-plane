CREATE TABLE IF NOT EXISTS mounts (
    id UUID PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    target TEXT NOT NULL,
    read_only BOOLEAN NOT NULL DEFAULT false,
    user_mountable BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mount_node (
    mount_id UUID NOT NULL REFERENCES mounts(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    PRIMARY KEY (mount_id, node_id)
);

CREATE TABLE IF NOT EXISTS egg_mount (
    mount_id UUID NOT NULL REFERENCES mounts(id) ON DELETE CASCADE,
    egg_id UUID NOT NULL REFERENCES server_templates(id) ON DELETE CASCADE,
    PRIMARY KEY (mount_id, egg_id)
);

CREATE TABLE IF NOT EXISTS mount_server (
    mount_id UUID NOT NULL REFERENCES mounts(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    PRIMARY KEY (mount_id, server_id)
);

CREATE INDEX IF NOT EXISTS mount_node_node_id_idx ON mount_node (node_id);
CREATE INDEX IF NOT EXISTS egg_mount_egg_id_idx ON egg_mount (egg_id);
CREATE INDEX IF NOT EXISTS mount_server_server_id_idx ON mount_server (server_id);
