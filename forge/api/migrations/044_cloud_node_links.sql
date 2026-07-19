CREATE TABLE IF NOT EXISTS cloud_node_links (
    provider TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, instance_id)
);

CREATE INDEX IF NOT EXISTS cloud_node_links_node_id_idx ON cloud_node_links (node_id);
