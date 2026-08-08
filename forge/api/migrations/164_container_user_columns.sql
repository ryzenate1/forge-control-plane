-- Add per-server container user (uid/gid) so workloads can run as a
-- specific user (e.g. itzg/minecraft-server expects uid 1000 for /data).
ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS container_uid INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS container_gid INTEGER NOT NULL DEFAULT 0;
