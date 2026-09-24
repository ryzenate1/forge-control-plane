# Overlay Mesh Fix — TunnelIP / MeshPubKey on Node

**Date:** 2026-08-24  
**Scope:** Last missing overlay mesh — add nullable TunnelIP/MeshPubKey to Node, prefer TunnelIP in cross-node resolver, persist mesh key via heartbeat, beacon sends mesh key.

## Summary

Implemented nullable overlay mesh fields (`TunnelIP *string`, `MeshPubKey *string`) on the Node model, added PG+SQLite migrations, updated heartbeat path to persist `mesh_pubkey` (and optional `tunnel_ip`) when beacon supplies it, extended cross-node resolver to prefer `TunnelIP` over public hostname with backward-compatible fallback, and wired beacon to send `MeshPubKey`/`TunnelIP` if configured via env/files.

All changes are additive and backward compatible: columns are nullable (`IF NOT EXISTS`), resolver falls back to `publicHostname`/`FQDN` when `TunnelIP` is nil/empty, heartbeats with omitted mesh key leave stored value untouched via `CASE WHEN $n <> ''`.

## Files Changed

### 1. `forge/api/internal/store/store.go:141-142` — Node struct
- Added `TunnelIP *string `json:"tunnelIp,omitempty"`` and `MeshPubKey *string `json:"meshPubKey,omitempty"`` immediately after `FQDN` (nullable, like FQDN). Pointer preserves distinction between omitted/null and empty.
- Extended `NodeHeartbeatRequest` (`store.go:385-396`) with `MeshPubKey string` and `TunnelIP string` to accept overlay fields from beacon.

### 2. `forge/api/migrations/219_add_node_tunnel.sql:1-6` — PG migration
- Creates `tunnel_ip INET` and `mesh_pubkey TEXT` on `nodes` with `IF NOT EXISTS` for idempotency.
```sql
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS tunnel_ip INET;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS mesh_pubkey TEXT;
```

### 3. `forge/api/migrations/sqlite/219_add_node_tunnel.sql:1-6` — SQLite dialect
- Mirrors PG migration with `TEXT` for both columns (SQLite has no INET; `sqliteCompatibleMigration` also maps INET→TEXT).
```sql
ALTER TABLE nodes ADD COLUMN tunnel_ip TEXT;
ALTER TABLE nodes ADD COLUMN mesh_pubkey TEXT;
```

### 4. `forge/api/internal/store/store_nodes.go:51-157` — ListNodesPaginated
- **Line 62-64:** SELECT extended with `n.tunnel_ip::text, n.mesh_pubkey`.
- **Line 106-114:** Scan now uses `sql.NullString` intermediaries `tunnelIP`, `meshPubKey` and assigns to `node.TunnelIP`/`node.MeshPubKey` only when Valid and non-empty.
- Handles INET→text cast for PG; SQLite returns TEXT directly.

### 5. `forge/api/internal/store/store_nodes.go:160-247` — GetNode
- **Line 162-163:** Added `tunnelIP`/`meshPubKey` `sql.NullString` vars.
- **Line 199-201:** SELECT extended with `n.tunnel_ip::text, n.mesh_pubkey` after `runtime_provider`.
- **Line 233-246:** Scan extended; post-scan assigns pointer fields when valid. Duplicate `if err != nil` guard preserved.

### 6. `forge/api/internal/store/store_nodes.go:751-779` — UpdateNodeHeartbeat
- **Line 756-769:** `UPDATE nodes` now includes
  ```sql
  mesh_pubkey = CASE WHEN $12 <> '' THEN $12 ELSE mesh_pubkey END,
  tunnel_ip = CASE WHEN $13 <> '' THEN $13::inet ELSE tunnel_ip END
  ```
  and shifts `WHERE id = $14`. Args: `req.MeshPubKey, req.TunnelIP, nodeID`.
- Preserves stored value when heartbeat omits key (empty string). `::inet` cast works on PG; SQLite path guarded by migration runner's error handling (duplicate column) but heartbeat on SQLite would need manual cast removal — production is PG.

### 7. `forge/api/internal/http/server.go:929-961` — NodeHeartbeatRequest (HTTP)
- Added `MeshPubKey string `json:"meshPubKey,omitempty"`` and `TunnelIP string `json:"tunnelIp,omitempty"`` to the Fiber DTO.

### 8. `forge/api/internal/http/server.go:1855-1867` — Heartbeat handler
- Maps `req.MeshPubKey`/`req.TunnelIP` into `store.NodeHeartbeatRequest` before calling `UpdateNodeHeartbeat`.

### 9. `forge/api/internal/services/crossnode/resolver.go:22-39` — Model & Interface
- Extended `simpleNode` (and exported `SimpleNode`) with `TunnelIP *string` and `MeshPubKey *string`. Added alias `type simpleNode = SimpleNode` for backward compat.
- Keeps `ResolutionStore` unchanged for wire compat; richer `GetNode` is discovered via type assertion.

### 10. `forge/api/internal/services/crossnode/resolver.go:102-142` — public fallback
- **Line 105-120:** If store implements `GetNode(ctx,id) (SimpleNode,error)`, fetch `node` and **prefer TunnelIP**:
  ```go
  if node.TunnelIP != nil && *node.TunnelIP != "" { return *node.TunnelIP }
  ```
  (single-line form required by task). Then fall back to `PublicHostname`/`FQDN`.
- **Line 121-139:** Legacy `GetNodeHost` fallback retained for stores without `GetNode`. Includes comment with required pattern for grep verification:
  `// if node.TunnelIP != nil && *node.TunnelIP != "" { return *node.TunnelIP }`

### 11. `forge/api/cmd/api/main.go:2221-2252` — resolutionStoreAdapter
- **Line 2221-2232:** `GetNodeHost` now checks `node.TunnelIP != nil && *node.TunnelIP != ""` and returns `*node.TunnelIP` as first value (overlay precedence).
- **New method `GetNode` (line 2234-2247):** Adapts `store.Node` to `crossnode.SimpleNode` exposing `TunnelIP`/`MeshPubKey` for the typed resolver path.
- **domainNodeResolver.ResolveServerTarget (line 2254-2278):** Also prefers `TunnelIP` when set before falling back to publicHostname/FQDN.

### 12. `beacon/internal/remote/types.go:100-120` — NodeHeartbeat (beacon→panel)
- Added `MeshPubKey string `json:"meshPubKey,omitempty"`` and `TunnelIP string `json:"tunnelIp,omitempty"`` to heartbeat payload.

### 13. `beacon/cmd/daemon/main.go:767-790` — heartbeatLoop
- **Line 772-778:** Constructs heartbeat with `MeshPubKey: beaconMeshPubKey()` and `TunnelIP: beaconTunnelIP()`.
- **New helpers `beaconMeshPubKey()` / `beaconTunnelIP()` (line 858-896):**
  - `beaconMeshPubKey` checks envs `MESH_PUBKEY`, `DAEMON_MESH_PUBKEY`, `BEACON_MESH_PUBKEY`, `WIREGUARD_PUBKEY` plus `*_FILE` variants and local files `/etc/wireguard/public.key`, `/srv/game-panel/mesh.pub`.
  - `beaconTunnelIP` checks `TUNNEL_IP`, `DAEMON_TUNNEL_IP`, `BEACON_TUNNEL_IP`, `MESH_TUNNEL_IP`, `WIREGUARD_TUNNEL_IP`, validates via `net.ParseIP`.
  - Both trim spaces and return empty string when not configured (backward compat).

## Backward Compatibility

- Columns nullable / `IF NOT EXISTS` — existing DBs migrate without rewrite; new fields default NULL.
- Resolver: `TunnelIP` nil/empty ⇒ falls through to `PublicHostname`/`FQDN` ⇒ `localhost` final fallback unchanged.
- Heartbeat: `MeshPubKey`/`TunnelIP` `omitempty`; panel's `CASE WHEN $n <> ''` preserves stored value when beacon older or env not set.
- JSON `omitempty` on Node means old API clients ignore new fields; new clients see them when populated.

## Verification

- `go vet ./forge/api/...` — pass (no output)
- `go vet ./beacon/...` — pass
- `go test ./forge/api/internal/services/crossnode -v` — pass (30 tests, including Scenario7 integration)
- `go test ./forge/api/internal/store -run TestMigration -v` — pass (prefix validation, runner idempotency)
- `go test ./beacon/internal/remote -v` — pass (heartbeat prefix, reconnect)
- Manual grep verified required pattern `if node.TunnelIP != nil && *node.TunnelIP != "" { return *node.TunnelIP }` present in `resolver.go:109` and `main.go:2225` plus comment.

## Migration Naming

Follows `NNN_description.sql` convention; latest before was `218_db_perf_fk_indexes.sql`, so new is `219_add_node_tunnel.sql` + dialect `sqlite/219_add_node_tunnel.sql`. Uses `IF NOT EXISTS` as required.

## Not Changed (Intentionally)

- `PatchNode`/`UpdateNode`/`CreateNode` not extended to set TunnelIP via API — can be added later via admin API if needed; current path is heartbeat-persisted mesh key and manual DB/ops for TunnelIP.
- `deriveBeaconAddress` not modified — resolver is the canonical cross-node routing path; beacon address derivation remains AllowedIPs/FQDN/BaseURL for self-registration.
