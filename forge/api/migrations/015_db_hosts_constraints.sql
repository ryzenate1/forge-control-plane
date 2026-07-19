-- Unique constraint on database host name per node (idempotent via CREATE UNIQUE INDEX IF NOT EXISTS)
CREATE UNIQUE INDEX IF NOT EXISTS database_hosts_name_node_id_uniq ON database_hosts (name, COALESCE(node_id, '00000000-0000-0000-0000-000000000000'::uuid));
