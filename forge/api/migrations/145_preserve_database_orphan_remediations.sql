ALTER TABLE database_orphan_remediations
    DROP CONSTRAINT IF EXISTS database_orphan_remediations_server_database_id_fkey,
    DROP CONSTRAINT IF EXISTS database_orphan_remediations_server_id_fkey,
    DROP CONSTRAINT IF EXISTS database_orphan_remediations_database_host_id_fkey;
