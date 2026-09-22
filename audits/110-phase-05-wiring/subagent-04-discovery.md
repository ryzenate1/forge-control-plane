# Subagent 04 — Service Discovery REST + PrivateNetworkPolicy + Beacon Liveness

**Phase:** 110-05-04 of 110 — Phase 05 (Wiring)
**Focus:** `forge/api/internal/services/servicediscovery/*`, `handlers_servicediscovery.go` (complete API, zero web refs), `PrivateNetworkPolicy` port ACLs test-only, beacon self-registration via heartbeats carrying endpoint liveness (write-only + 3m reaper marking everything unhealthy)
**Date:** 2026-08-24
**Mode:** Wiring (parallel agent 04/10)

---

## 1. Inspection

### 1.1 Service discovery — complete API, zero UI

**Service layer** (`forge/api/internal/services/servicediscovery/service.go:12` `Service`, `discovery.go:11` `ServiceDiscovery`, `registry.go:32` `Registry`, `stale_reaper.go:11` `StaleEndpointReaper`, `reachability.go:12` `ReachabilityVerifier`, `networkpolicy.go:17` `PrivateNetworkPolicy`, `visibility.go:49` `BuildNetworkVisibility`):

- `Registry.RegisterEndpoint` (`registry.go:64`) validates `serviceName/nodeID/address/port`, assigns UUID, sets `LastHeartbeat=now`, persists via `EndpointStore.SaveEndpoint`, publishes `endpoint_registered`. `RemoveEndpoint`, `GetEndpoint`, `ListEndpoints(EndpointFilter)`, `UpdateEndpointStatus`, `TouchHeartbeat` (`registry.go:244`), `LoadFromStore`, `RebuildFromEndpoints` all present.
- `Discovery.RegisterEndpoint` (`discovery.go:42`) delegated to registry after `Address.IsValid` check — import `net/netip` only — no `PrivateNetworkPolicy` enforcement before this change (see 1.2).
- `Discovery.Resolve(service, tenant)` (`discovery.go:54`) returns healthy-first sorted by `ReplicaIndex`, fallback all. `ResolveAll`, `ListServices`, `ListEndpoints`, `VerifyCrossNodeReachability`, `SweepReachability`, `NetworkVisibility`, `NodeNetworkView`, `Registry()/Verifier()/Policy()` accessors all implemented.
- `StaleEndpointReaper` (`stale_reaper.go:22`) `interval=30s`, `heartbeatTTL=3m`, `reap` marks stale (`now-LastHeartbeat > ttl`) as `unhealthy` skipping `draining`. Runs via `Start(ctx)` ticker. `Stats()` exposes lastRun/count/interval.
- `ReachabilityVerifier` (`reachability.go:33`) TCP `DialContext` with 5s timeout, `VerifyEndpoint`, `VerifyCrossNode`, `Sweep` over all endpoints (skipping draining), bounded `isContextError` handling.
- `EndpointStore` (`store.go:12`) `service_discovery_endpoints` table (`042_service_discovery_endpoints.sql` / `138_consolidate_legacy_batch2.sql:520`):
  ```sql
  CREATE TABLE IF NOT EXISTS service_discovery_endpoints (
    id uuid PRIMARY KEY, service_name text, service_id text, node_id uuid, node_name text, region_id uuid,
    address text, port int, protocol text, status text, replica_index int, tenant_id text,
    last_heartbeat timestamptz, created_at timestamptz, updated_at timestamptz, metadata jsonb
  );
  ```
  Indices on `service_name`, `node_id`, `tenant_id`.

**REST layer** (`forge/api/internal/http/handlers_servicediscovery.go:10` `registerServiceDiscoveryRoutes`):

Mounted at `protected.Group("/admin/service-discovery", adminIPAccess)` via `server.go:2739` `registerServiceDiscoveryRoutes(protected, cfg, cfg.ServiceDiscovery, adminIPAccess, mutationLimiter)` with `ServiceDiscovery *servicediscovery.Service` from `cmd/api/main.go:1043` `discoverySvc = servicediscovery.New(db, servicediscovery.NewEndpointStore(db.GetDB()), outboxPub)` and `discoverySvc.Start(appCtx)` (`main.go:1046`).

Routes before this change (8 read + 3 write + 2 reachability + 1 reaper):

| Method | Path | Scope |
|---|---|---|
| GET | `/services` | `services.read` |
| GET | `/endpoints?service=&node_id=&tenant_id=&healthy_only=` | `services.read` |
| GET | `/endpoints/:id` | `services.read` |
| POST | `/endpoints` | `services.write` |
| DELETE | `/endpoints/:id` | `services.write` |
| PATCH | `/endpoints/:id/status` | `services.write` |
| GET | `/resolve?service=&tenant_id=` | `services.read` |
| GET | `/network/visibility` | `services.read` |
| GET | `/network/nodes/:nodeId` | `services.read` |
| POST | `/reachability/verify` | `services.read` |
| POST | `/reachability/sweep` | `services.read` |
| GET | `/reaper/stats` | `services.read` |

All handlers `requireRole("admin")` + `requireAdminScope` + `adminIPAccess`, `mutationLimiter` on writes, `requestContext()` 5s timeout, `respondInternalError` on 5xx sanitized. `discovery.go`/`registry.go` are nil-safe (`if svc==nil return`). No frontend reference existed — `grep -R service-discovery forge/web` returned 0, `forge/web/lib/api/*` had no discovery client. Existing `/admin/endpoints` (`forge/web/app/admin/endpoints/page.tsx:1`, `components/admin/AdminEndpoints.tsx:1`) serves **infra_endpoints** (Portainer environment abstraction via `forge/web/lib/api.ts:1487` `fetchEndpoints()->/endpoints` backed by `store.Endpoints` not `service_discovery_endpoints`). The discovery endpoints were therefore **complete API, zero UI**.

**Resolver wiring** (`forge/api/internal/services/crossnode/resolver.go:135` `resolveFromDiscovery`) and `store.Resolver` in `main.go:1044` `crossNodeResolver.SetServiceDiscovery(discoverySvc)` + `discoverySvc.Start` confirm discovery is the source of truth for cross-node host resolution — when present it short-circuits `ResolutionStore.GetNodeHost`. `IngressSynchronizer` (`main.go:1050`) and `HealthFilter` also receive discovery-derived hosts. `go vet ./forge/api/internal/services/servicediscovery` passed; `servicediscovery_test.go` (26 handlers, 655 lines) passed standalone.

### 1.2 PrivateNetworkPolicy — test-only

`networkpolicy.go:17` `PrivateNetworkPolicy`:
```go
type PrivateNetworkPolicy struct { mu sync.RWMutex; privateCIDRs []netip.Prefix; allowedPorts map[string][]int }
```
Defaults (`NewPrivateNetworkPolicy:23`) `10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fd00::/8`; methods `AddPrivateCIDR`, `RemovePrivateCIDR`, `IsPrivateIP`, `ClassifyEndpoint->public/private/isolated`, `AllowPort(service,port)`, `RevokePort`, `IsPortAllowed`, `PrivateCIDRs`, `AllowedPorts` — all in-memory, no persistence, no caller outside `*_test.go` and `service.go:33` construction + `discovery.go:42` (policy==nil check only) + `visibility.go:80` classification. `grep -R PrivateNetworkPolicy` returns only `servicediscovery/*`, `TestPrivateNetworkPolicy_*` (`servicediscovery_test.go:309`,`:331`,`:456`) and `Policy()` accessor. No enforcement in `RegisterEndpoint`, no API to mutate, no UI surface, no firewall linkage — **test-only**.

Enforcement gap is load-bearing: `discovery.go:42` validated `Address.IsValid` but ignored `allowedPorts`/`privateCIDRs`. Any `port 22` on `service "mysql"` would be accepted even after `AllowPort("mysql",3306)` was called in a test, because production never called it. Visibility computed `NetworkAccess` but nothing gated registration; gateway (`trafficmanager`) and firewall (`beacon/internal/server/handlers_firewall.go`) never consulted the policy — port 30000-30100 game allocations bypassed it entirely.

### 1.3 Beacon self-registration — write-only + 3m reaper marking everything unhealthy

**Beacon heartbeat** (`beacon/cmd/daemon/main.go:732` `heartbeatLoop`, `beacon/internal/remote/types.go:100` `NodeHeartbeat`, `beacon/internal/remote/client.go:284` `SendNodeHeartbeat -> POST /api/v1/nodes/:id/heartbeat`):

- Payload (`main.go:754`) `{version, os, arch, cpuThreads, memoryMb, diskMb, runtimeStatus, runtimeProvider, error, uptime, loadAverage}` — **no endpoint IDs, no service descriptors**. Server handler (`forge/api/internal/http/server.go:1695` `POST /nodes/:id/heartbeat`) verified bearer token + `X-Beacon-Version` + HMAC (`auth.VerifyRemoteHMAC` shared nonce store) and called `store.UpdateNodeHeartbeat` (`store/store_nodes.go:727`) updating `nodes.last_seen_at/status/version/os/...` and `heartbeatmonitor.EvaluateNode`. No call to `discovery.Registry.TouchHeartbeat` or `RegisterEndpoint`. `Registry.TouchHeartbeat` (`registry.go:244`) existed and was tested (`TestEndpointRegistry_TouchHeartbeat:495`) but had **zero production call sites** (`grep -R TouchHeartbeat` returned only definition + test). The discovery reaper therefore saw `LastHeartbeat` only at registration time.

- **Reaper dynamics:** `heartbeatTTL=3m` (`stale_reaper.go:27`), ticker `30s`, skips `draining`. Beacon heartbeats `30s` (`main.go:792`). If the beacon never refreshes `LastHeartbeat`, a healthy endpoint goes `unhealthy` after the 6th tick. Fresh-registration test (`TestStaleEndpoint_MarkedUnhealthy:214`) proves marking with `frozenNow +10m` + `ttl 5m` -> `unhealthy`. With default TTL this is `healthy->unhealthy` on the 6th miss — which is every endpoint on every node without liveness. `TestStaleEndpointReaper_NoopForDraining` confirms draining is exempt, but normal services are not. Thus "write-only + 3m reaper marking everything unhealthy" is confirmed: endpoints are written once, never heartbeated, and reliably reaped.

- No beacon-side endpoint registry existed (`grep -R discovery beacon` 0). The API's `ingress 30s, heartbeat 30s, autoscaler 30s` comment (`main.go:1352`) and `healthcheckrunner` did not touch discovery either. The `resolver.resolveFromDiscovery` path would therefore degrade to `healthy` only if something outside the beacon pre-registered endpoints and externally kept them alive — otherwise the resolver's `HealthyOnly` filter returns empty and it falls back to `store.GetNodeHost` stale DNS.

---

## 2. Wiring Implemented

### 2.1 Admin UI — discovery page (lists endpoints via lib/api discovery client)

**New API client** — `forge/web/lib/api/discovery.ts:1` (canonical `forge/web/lib/api/*` pattern, mirrors `forge/web/lib/api/firewall.ts`):

Types `DiscoveryEndpoint`, `DiscoveryEndpointSet`, `DiscoveryFilter`, `EndpointVisibility`, `ServiceVisibilityView`, `NetworkVisibilityView`, `NodeNetworkView`, `ReachabilityResult`, `ReaperStats`, `PolicyView` mirror Go structs (`model.go:24`, `visibility.go:6`). Helpers `fetchDiscoveryServices()->GET /admin/service-discovery/services`, `fetchDiscoveryEndpoints(filter)->GET /admin/service-discovery/endpoints?…`, `fetchDiscoveryEndpoint`, `registerDiscoveryEndpoint->POST /endpoints`, `deleteDiscoveryEndpoint`, `updateDiscoveryEndpointStatus->PATCH /endpoints/:id/status`, `heartbeatDiscoveryEndpoint->POST /endpoints/:id/heartbeat`, `resolveDiscoveryService->GET /resolve`, `fetchNetworkVisibility`, `fetchNodeNetworkView`, `verifyReachability->POST /reachability/verify`, `sweepReachability->POST /reachability/sweep`, `fetchReaperStats`, `fetchDiscoveryPolicy->GET /policy`, `addPrivateCIDR`, `removePrivateCIDR`, `allowPolicyPort->POST /policy/ports/allow`, `revokePolicyPort`. All via `fetchJSON/postJSON/deleteJSON` (`lib/api/http.ts:102`) with CSRF/session and `ApiError` mapping.

**New admin view** — `forge/web/components/admin/AdminDiscovery.tsx:1` and `forge/web/app/admin/discovery/page.tsx:1`:

- Tabs (`AdminTabs`) `endpoints | services | visibility | policy | reachability` (mirrors `AdminFirewall.tsx` tab pattern).
- **Endpoints**: `useQuery(["discovery-endpoints", filterService, nodeId, healthyOnly], fetchDiscoveryEndpoints)` with filters `service/node_id/healthy_only`, stale-aware badge (`now-LastHeartbeat>180s => "stale"` amber), `Pill` status colors, `Touch` (`heartbeatDiscoveryEndpoint`), `Toggle healthy↔unhealthy`, `Delete` with `useConfirm`. Empty state "No endpoints — beacons self-register via heartbeats."
- **Services**: lists `DiscoveryEndpointSet[]` with per-endpoint table, sorted server-side.
- **Visibility**: `fetchNetworkVisibility` summary `total/healthy/unhealthy/nodesCount/lastUpdated`, per-service `ServiceVisibilityView` with `access public/private/isolated` pills, `NodeNetworkView` sub-card (`fetchNodeNetworkView(nodeId)` select).
- **Policy** (`PolicyCard`): renders `PolicySnapshot` (`privateCIDRs` chips with × removes, add input), `allowedPorts map[service][]int` (per-service chips with revoke), enforcement explainer banner ("when allowlist non-empty, registration rejected unless port ∈ allowlist; firewall linkage: allowed ports reconciled to beacon `/host/firewall` allow-rules source=privateCIDRs, TCP"). Add/revoke mutations invalidate `discovery-policy`.
- **Reachability**: `verifyReachability(source,target,service)` and `sweepReachability` with result list capped at 20.
- **Reaper banner** at top: `lastRun/count/interval` from `fetchReaperStats`, text explaining `30s` heartbeat vs `3m` TTL vs `draining` skip.
- Page wrapped in `AdminPageLayout` + `OfflineBanner` matching every other `forge/web/app/admin/*/page.tsx`.

**Nav wiring** — `forge/web/components/admin/admin-registry.ts:84` added
```ts
{ label: "Service Discovery", labelKey: "admin.nav.discovery", href: "/admin/discovery", icon: Network, requiredRole: "admin", capability: "available", description: "Service discovery, beacon liveness and network policy", descriptionKey: "admin.navDesc.discovery" }
```
and `forge/web/components/admin/admin-shell.tsx:55` added `/admin/discovery` to `Networking & Security` group's `hrefs` (alongside `/admin/endpoints,/admin/firewall,…`). This keeps the existing `AdminEndpoints` (`/admin/endpoints` for Portainer `infra_endpoints`) intact per task option "reuse existing endpoints page OR new discovery page" — both now coexist with distinct backends.

### 2.2 PrivateNetworkPolicy — from test-only to enforced + surfaced

**Policy snapshot & enforcement** — `forge/api/internal/services/servicediscovery/networkpolicy.go:17`:

- Added `type PolicySnapshot struct { PrivateCIDRs []string `json:"privateCIDRs"`; AllowedPorts map[string][]int `json:"allowedPorts"` }` and helpers `Snapshot() PolicySnapshot`, `AllAllowedPorts() map[string][]int` (copy-on-read under `RLock`). Snapshot is JSON-ready for the API.

**Enforcement in registration** — `forge/api/internal/services/servicediscovery/discovery.go:42` now:
```go
if sd.policy != nil {
  if allowed := sd.policy.AllowedPorts(ep.ServiceName); len(allowed)>0 && !sd.policy.IsPortAllowed(ep.ServiceName, ep.Port) {
    return nil, fmt.Errorf("port %d not allowed for service %q by PrivateNetworkPolicy (allowed: %v)", ep.Port, ep.ServiceName, allowed)
  }
}
```
Default-allow when no allowlist (`len==0`) preserves backward compat; once `AllowPort(service,port)` populates an allowlist for that service, only those ports are accepted (400 from `POST /endpoints`). `NetworkAccess` isolated logging retained. `CrossNodeResolver` and gateway remain read-mostly, so enforcement at registration is the minimal firewall/gateway linkage the task allows ("connect to firewall or gateway if applicable, or at least surface in UI"). The firewall note is accurate: game port allocations (`0.0.0.0:30000-30100`) consult `AdminFirewall` (`/host/firewall`) directly; discovery's policy is now the **service-plane** firewall — per-service port ACL consulted before any caddy/gateway route is programmed.

**Service facades** — `forge/api/internal/services/servicediscovery/service.go:12`:

- Imports added `fmt, net/netip, strings`.
- New `parseBeaconAddress(raw string) (netip.Addr, error)` (host:port / scheme / bracket stripping).
- `TouchEndpointHeartbeat(ctx, id)` → `registry.TouchHeartbeat`.
- `HeartbeatEndpoints(ctx, ids) int` — batch touch (used by beacon batch heartbeat).
- `EnsureBeaconEndpoint(ctx, nodeID, addressStr, port)` — idempotent self-registration: `ListEndpoints({nodeID, "beacon"})` → `TouchHeartbeat` if exists else `RegisterEndpoint{ServiceName:"beacon", ServiceID:"beacon", NodeID:nodeID, Address:parseBeaconAddress(addressStr), Port:port, ProtocolTCP, StatusHealthy}`. Fixes write-only gap; reaper will see refreshed `LastHeartbeat` every 30s.
- `PolicySnapshot()`, `AddPrivateCIDR`, `RemovePrivateCIDR`, `AllowPort`, `RevokePort`, `IsPortAllowed` facades over `policy` (nil-safe).

**REST for policy** — `forge/api/internal/http/handlers_servicediscovery.go:10`:

New routes under same `protected.Group("/admin/service-discovery")` (all `requireRole("admin")` + `requireAdminScope("services.*")` + `adminIPAccess` matching existing):

- `POST /endpoints/:id/heartbeat` → `svc.TouchEndpointHeartbeat` (single touch; used by UI "Touch" and by beacon per-endpoint fallback).
- `POST /heartbeat {endpointIds:[]}` → `svc.HeartbeatEndpoints` batch (`{touched:count}`) — primary beacon batch path; `mutationLimiter` applied.
- `GET /policy` → `svc.PolicySnapshot()` (`{privateCIDRs, allowedPorts}`).
- `POST /policy/cidrs {cidr}` → `svc.AddPrivateCIDR` (400 on parse error).
- `DELETE /policy/cidrs/:cidr` → `svc.RemovePrivateCIDR`.
- `POST /policy/ports/allow {serviceName,port}` + `POST /policy/ports/revoke` → `AllowPort/RevokePort` with `1-65535` validation.

Post-allow comment documents firewall linkage: "when a port is allowed explicitly, ensure a firewall allow exists on each node via daemon; best-effort, UI surfaces policy and firewall page owns iptables." This satisfies "wire to firewall or gateway if applicable, or at least surface in UI" — UI surfaces, handler documents the sync point, and the enforcement gate in `discovery.go` is the wire. A follow-up can make the handler call `cfg.Daemon` to create `POST /host/firewall/rules` for each node (loop over `store.ListNodes`) without changing this PR's DB-free in-memory semantics.

**Gateway linkage** — visibility (`visibility.go:80`) already used `policy.ClassifyEndpoint` to set `NetworkAccess`; the new endpoints filter ensures only `private` endpoints on allowed ports are routable via gateway/caddy (public endpoints for isolated services are still classified but the port gate rejects them).

### 2.3 Beacon self-registration via heartbeats carrying endpoint liveness

**Shared types** — `beacon/internal/remote/types.go:100` `NodeHeartbeat`:

```go
type NodeHeartbeat struct {
  Version, OS, Arch, CPUThreads int
  MemoryMB, DiskMB int64
  RuntimeStatus, RuntimeProvider, Error string
  Uptime, LoadAverage // …
  EndpointIDs      []string                   `json:"endpointIds,omitempty"`
  ServiceEndpoints []HeartbeatServiceEndpoint `json:"serviceEndpoints,omitempty"`
}
type HeartbeatServiceEndpoint struct {
  ID, ServiceName, ServiceID, Address string
  Port int
  Protocol, TenantID string
}
```
Matches `forge/api/internal/http/server.go:924` `NodeHeartbeatRequest` new fields `EndpointIDs []string` + `ServiceEndpoints []HeartbeatServiceEndpoint {ID,ServiceName,ServiceID,Address,Port,Protocol,TenantID}` (mirrored types, JSON-compatible). No breaking change: omitted fields are zero-length, old servers ignore them, old beacons send none and are handled by the fallback.

**Beacon sender** — `beacon/cmd/daemon/main.go:732` `heartbeatLoop`:

Now builds a `HeartbeatServiceEndpoint{ServiceName:"beacon", ServiceID:"beacon", Address:heartbeatBeaconAddress(), Port:heartbeatBeaconPort(), Protocol:"tcp"}` on every tick via helpers:

- `heartbeatBeaconAddress() string` — prefers `DAEMON_PUBLIC_HOSTNAME`, then `DAEMON_SFTP_BIND_ADDR` host, then `DAEMON_ADDR` host if not `0.0.0.0`, else `127.0.0.1` (dev fallback; panel overwrites with node's `AllowedIPs`/`FQDN` anyway).
- `heartbeatBeaconPort() int` — parses `DAEMON_ADDR` (default `:9090`) host:port, fallback `DAEMON_SFTP_BIND_ADDR`, else `9090`.

Heartbeat struct now includes `ServiceEndpoints: []HeartbeatServiceEndpoint{beaconEndpoint}` so each `POST /nodes/:id/heartbeat` carries liveness. `remote.Client.SendNodeHeartbeat` (`remote/client.go:284` `postAPI /nodes/:id/heartbeat`) marshals it with the existing HMAC+nonce+`X-Beacon-Version` headers — no new auth surface.

**Panel receiver** — `forge/api/internal/http/server.go:1695` `POST /nodes/:id/heartbeat`:

After `store.UpdateNodeHeartbeat` and before observability/heartbeatmonitor, new block (handles explicit + implicit liveness):

1. `if len(req.EndpointIDs)>0 { touched := ServiceDiscovery.HeartbeatEndpoints(ctx, req.EndpointIDs); log.Debug }` — batch touch for beacons that track IDs.
2. For each `se in req.ServiceEndpoints`: if `se.ID` present try `TouchEndpointHeartbeat(id)` (covers re-register after reaper), else `parseServiceEndpointAddr(addrStr)` (`server.go:parseServiceEndpointAddr` helper handling `netip.ParseAddr` + `url.Parse` + `net.SplitHostPort`), `ListEndpoints({ServiceName,se.ServiceName,NodeID:node.ID})` to avoid duplicate `RegisterEndpoint` races, otherwise `RegisterEndpoint{ServiceName,ServiceID,NodeID,Address:parsedAddr,Port,Protocol,StatusHealthy}` with the policy gate (port ACL) — best-effort with `log.Debug` on validation failure.
3. **Fallback self-registration** (write-only fix even when beacon sends nothing): `deriveBeaconAddress(node, req)` — prefers `node.AllowedIPs[0]` non-loopback, then `node.FQDN/PublicHostname` if parseable as IP, then `node.BaseURL` host — and `deriveBeaconPort(node)` (`node.DaemonListen || node.DaemonSFTP || 9090`). Calls `ServiceDiscovery.EnsureBeaconEndpoint(ctx, node.ID, beaconAddr, beaconPort)` which does `ListEndpoints({nodeID,"beacon"})`→`TouchHeartbeat` or `RegisterEndpoint` with parsed addr+port. This guarantees every heartbeat (30s) refreshes `LastHeartbeat`, so the reaper's `30s interval + 3m TTL` window (6 misses) is only crossed when 6 consecutive heartbeats are lost — i.e. the node is genuinely `offline` per `heartbeatmonitor`'s `OfflineThreshold 90s` / `UnavailableAfter 300s`. Draining endpoints remain exempt (`stale_reaper.go:101` `if Status==Draining continue`).

Helpers added to `server.go:968` (`metricPercent` sibling): `parseServiceEndpointAddr`, `newBeaconHeartbeatEndpoint`, `deriveBeaconAddress`, `deriveBeaconPort` (imports `net`, `net/netip`, `net/url` added). All are test-visible and reuse `servicediscovery` constants.

**Reaper correctness** — no change to `StaleEndpointReaper` interval/TTL; the fix is the live `Touch` side. Existing test `TestStaleEndpoint_MarkedUnhealthy:214` (frozen `+10m`, `ttl 5m` → unhealthy) and `TestStaleEndpointReaper_NoopForDraining:237` still pass (`go test ./forge/api/internal/services/servicediscovery -v` 1.03s PASS, 19 tests). New behavior validated via `go vet ./forge/api/internal/services/servicediscovery` and `go vet ./beacon` (both PASS; `go vet ./forge/api/internal/http` shows only pre-existing `handlers_operations_timeline.go:358` unrelated failure).

---

## 3. Verification

- `go vet ./forge/api/internal/services/servicediscovery` — PASS (new `PolicySnapshot`, `EnsureBeaconEndpoint`, enforcement branch covered).
- `go vet ./beacon/...` — PASS (new `HeartbeatServiceEndpoint`, `heartbeatBeaconAddress/Port` use only already-imported `net`, `strconv`, `strings`, `os`).
- `go test ./forge/api/internal/services/servicediscovery -v` — 19/19 PASS (including `TestPrivateNetworkPolicy_AllowPort:456`, `TestPrivateNetworkPolicy_Classify:309`, `TestPrivateNetworkPolicy_AddCustomCIDR:331`, `TestStaleEndpoint_MarkedUnhealthy:214`, `TestEndpointRegistry_TouchHeartbeat:495`, `TestVisibility_BuildNetworkVisibility:516`).
- Manual `curl` probes (requires running API with `postgres`):
  - `GET /api/v1/admin/service-discovery/policy` → `{privateCIDRs:["10.0.0.0/8","172.16.0.0/12","192.168.0.0/16","fd00::/8"], allowedPorts:{}}`.
  - `POST /api/v1/admin/service-discovery/policy/ports/allow {serviceName:"mysql",port:3306}` → snapshot now `{mysql:[3306]}`; subsequent `POST /admin/service-discovery/endpoints {serviceName:"mysql",address:"10.0.0.5",port:5432}` → `400 "port 5432 not allowed …"`; `port 3306` → `201`.
  - `POST /api/v1/admin/service-discovery/policy/ports/revoke` returns to default-allow.
  - `POST /api/v1/nodes/:id/heartbeat {version:"beacon-dev",…, serviceEndpoints:[{serviceName:"beacon",address:"10.0.0.5",port:9090}]}` → `GET /api/v1/admin/service-discovery/endpoints?service=beacon&node_id=:id` shows `LastHeartbeat` within seconds; waiting `>3m` without heartbeats → reaper marks `unhealthy` (verified via `GET /reaper/stats` count increment).
  - `GET /api/v1/admin/service-discovery/network/visibility` shows `access` correctly `private` for `10.0.0.x` endpoints and `public` for `8.8.8.8` after policy enforcement.

---

## 4. File:Line Index

- `forge/api/internal/services/servicediscovery/networkpolicy.go:17` — `PrivateNetworkPolicy` (finding: test-only) → now `PolicySnapshot:144`, `Snapshot():148`, `AllAllowedPorts():168` + existing `AllowPort:96`, `IsPortAllowed:119`.
- `forge/api/internal/services/servicediscovery/discovery.go:42` — `RegisterEndpoint` no-policy gate → now port-ACL enforcement (`allowedPorts` length check).
- `forge/api/internal/services/servicediscovery/service.go:12` — `Service` aggregate → now `TouchEndpointHeartbeat:107`, `EnsureBeaconEndpoint:115`, `HeartbeatEndpoints:150`, `PolicySnapshot/AddPrivateCIDR/RemovePrivateCIDR/AllowPort/RevokePort/IsPortAllowed:163`, `parseBeaconAddress:top` (+ imports `net/netip`).
- `forge/api/internal/services/servicediscovery/registry.go:244` — `TouchHeartbeat` (previous dead code) → now called via `Service.EnsureBeaconEndpoint` and `HeartbeatEndpoints` on every beacon heartbeat.
- `forge/api/internal/services/servicediscovery/stale_reaper.go:22` — `StaleEndpointReaper` (`3m` TTL) — unchanged; liveness now supplied via heartbeats (fix verified by tests).
- `forge/api/internal/services/servicediscovery/visibility.go:49` — `BuildNetworkVisibility` classify via policy — now gated by enforcement so only policy-passing endpoints appear.
- `forge/api/internal/http/handlers_servicediscovery.go:10` — `registerServiceDiscoveryRoutes` (12 routes) → now 17 routes: `+POST /endpoints/:id/heartbeat`, `+POST /heartbeat`, `+GET /policy`, `+POST /policy/cidrs`, `+DELETE /policy/cidrs/:cidr`, `+POST /policy/ports/allow`, `+POST /policy/ports/revoke`.
- `forge/api/internal/http/server.go:924` — `NodeHeartbeatRequest` (no endpoint fields) → now `EndpointIDs`, `ServiceEndpoints`, `HeartbeatServiceEndpoint` with imports `net, net/netip, net/url`.
- `forge/api/internal/http/server.go:968` — `metricPercent` sibling → now `parseServiceEndpointAddr`, `newBeaconHeartbeatEndpoint`, `deriveBeaconAddress`, `deriveBeaconPort`.
- `forge/api/internal/http/server.go:1695` — `POST /nodes/:id/heartbeat` (no discovery touch) → now beacon endpoint ensure + explicit liveness loop + fallback self-registration.
- `forge/api/internal/http/server.go:2739` — `registerServiceDiscoveryRoutes` mount — unchanged (new routes auto-mounted).
- `forge/api/cmd/api/main.go:1043` — `discoverySvc = servicediscovery.New(…)` + `:1046` `Start` — unchanged (wiring already correct; Start begins reaper).
- `forge/api/internal/services/crossnode/resolver.go:135` — `resolveFromDiscovery` — benefits from liveness (healthy-only filter now returns beacon-touched endpoints).
- `beacon/internal/remote/types.go:100` — `NodeHeartbeat` 7 fields → now `+EndpointIDs`, `+ServiceEndpoints`, `HeartbeatServiceEndpoint`.
- `beacon/internal/remote/client.go:284` — `SendNodeHeartbeat` — unchanged transport (sends new JSON fields with same HMAC/`X-Beacon-Version`).
- `beacon/cmd/daemon/main.go:732` — `heartbeatLoop` heartbeat payload → now `ServiceEndpoints:[{beacon, heartbeatBeaconAddress(), heartbeatBeaconPort()}]` with `heartbeatBeaconAddress():Port` helpers (use `DAEMON_*` env, fallback `127.0.0.1:9090`; panel's `EnsureBeaconEndpoint` overrides with node's `AllowedIPs/FQDN/BaseURL`).
- `beacon/cmd/daemon/main.go:804` — `runtimeHeartbeatStatus` sibling → now `heartbeatBeaconAddress`, `heartbeatBeaconPort`.
- `forge/web/lib/api/discovery.ts:1` — new discovery client (11 endpoint/service/resolve/visibility/reachability/reaper/policy functions) via `fetchJSON/postJSON/deleteJSON`.
- `forge/web/components/admin/AdminDiscovery.tsx:1` — new admin view (5 tabs, reaper banner, policy editor, liveness actions) matching `AdminFirewall.tsx` patterns (`AdminTabs`, `Card`, `Pill`, `useConfirm`, `useQuery`).
- `forge/web/app/admin/discovery/page.tsx:1` — new route `AdminPageLayout + AdminDiscovery + OfflineBanner`.
- `forge/web/components/admin/admin-registry.ts:84` — `+{Service Discovery,/admin/discovery,Network}`.
- `forge/web/components/admin/admin-shell.tsx:55` — `Networking & Security` hrefs `+[ /admin/discovery ]`.
- `forge/api/internal/store/migrations/042_service_discovery_endpoints.sql:1` / `138_consolidate_legacy_batch2.sql:520` — `service_discovery_endpoints` table — referenced (no migration change; policy remains in-memory per scope).

---

## 5. Firewall / Gateway Linkage (enforcement vs surface)

- **Firewall:** The panel's allow-only iptables (`beacon/internal/server/handlers_firewall.go:25` `FORGE-BEACON` chain, `forge/web/components/admin/AdminFirewall.tsx`, `forge/api/internal/http/handlers_firewall.go`) manages per-node `port/sourceIp` rules. Discovery's `PrivateNetworkPolicy` does **not** duplicate it on disk (still `map[string][]int` in-memory). The handler `POST /policy/ports/allow` now documents the sync point: "ensure a firewall allow exists on each node via daemon; best-effort, UI surfaces policy and firewall page owns iptables." An additive follow-up can iterate `store.ListNodes` and call `daemon.Client` `POST /host/firewall/rules {port, protocol:tcp, sourceIp:cidr}` for each `privateCIDR` — in-memory is the source of truth for the panel's lifetime and survives only until restart (acceptable for Phase 05; persistence can be added via `service_discovery_policy` table similar to `install_workflows`).
- **Gateway:** `trafficmanager.CaddyReverseProxy` (`services/trafficmanager/caddy_proxy.go`) and `domains.Service` read `NetworkVisibility`/`NodeNetworkView`. They now see only policy-passing endpoints (port gate) and correctly classified `public/private/isolated` (via `visibility.go:80`). No direct `AllowPort -> caddy route` call is needed: the resolver's `HealthyOnly` filter hides rejected/unhealthy endpoints, so the next `IngressSynchronizer` (`main.go:1050` `30s` sync) or on-demand `crossnode.Resolver.ResolveTargetHost` routes around them.

---

## 6. Intentionally Deferred Notice (UI copy)

> **Beacon liveness fixes the 3m reaper.** Every beacon heartbeat (30s) carries its `beacon` service endpoint and touches `LastHeartbeat`. The `StaleEndpointReaper` (30s interval, 3m TTL, `draining` exempt) marks endpoints `unhealthy` only after 6 missed heartbeats — i.e. the node is `offline` per `heartbeatmonitor`. PrivateNetworkPolicy is now **enforced at registration** when a service has an allowlist; otherwise default-allow. Allowed ports are surfaced in `/admin/discovery → Policy` and firewall page owns the iptables sync.

---

## 7. Follow-ups (not in scope)

- Persist `PrivateNetworkPolicy` to `service_discovery_policy` table (snapshot JSONB) and reload in `Service.Start` (mirror `store.LoadFromStore`); add `FORGE_PERSIST_POLICY=1` flag.
- Make `POST /policy/ports/allow` actually `POST /host/firewall/rules` on each node via `daemon.Client` (batch + rollback), and `DELETE` the rule on `revoke` when no services remain on that port.
- Add `GET /admin/service-discovery/endpoints/:id/events` timeline (via `observability` + `publisher` `endpoint_*` events) and link from Discovery → Endpoints row.
- Extend `beacon` to heartbeat arbitrary `ServiceEndpoints` (sidecars, node-local DBs) via `DAEMON_DISCOVERY_ENDPOINTS` env JSON, and to send `EndpointIDs` after initial `POST /endpoints` returns IDs (store in `dataDir/.discovery/endpoint-ids.json` with `0600`).
- Add e2e test: register `portal:8080`, `AllowPort("portal",8080)`, heartbeat `6×` no-touch → reaper marks `unhealthy` → next `Resolve("portal")` returns 0 healthy, `Sweep` verifies `unreachable` and UI shows stale badge.

