-- 219_add_node_tunnel.sql
-- Overlay mesh: tunnel IP (WireGuard/overlay) and mesh public key on nodes.
-- Both columns are nullable for backward compatibility; existing nodes continue
-- to route via publicHostname/FQDN when TunnelIP is NULL/empty.
-- IF NOT EXISTS makes the migration idempotent and safe for reruns.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tunnel_ip INET;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS mesh_pubkey TEXT;
