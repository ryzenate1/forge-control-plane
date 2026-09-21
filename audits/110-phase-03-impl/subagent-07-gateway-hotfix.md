# Subagent 07 — Gateway Phase A Hotfixes (Stop the Bleeding)

**Slice:** Gateway Phase A Hotfixes — no schema change
**Agent:** 110-03-07 of 20 (parallel slice)
**Date:** 2026-08-24
**Scope:** `crossnode/ingress_sync.go`, `trafficmanager/caddy_proxy.go`, `trafficmanager/traefik_proxy.go`, `trafficmanager/service.go`, `http/handlers_trafficmanager.go`
**Findings addressed:** F-NET-01, F-NET-02, F-NET-03, F-NET-04, F-NET-06, F-NET-07 (P0 cluster)

---

## 1. Summary

Implemented six no-schema hotfixes to stop the bleeding in the gateway cluster. All changes are in-memory/config-path only, no migrations. Total diff: 6 files, ~500 lines, 0 schema changes. Verified via `go test ./internal/services/trafficmanager` and `./internal/services/crossnode` (both PASS).

The densest P0 cluster (F-NET-01, F-NET-02, F-NET-03, F-NET-04, F-NET-06, F-NET-07) is now mitigated at the hotfix level; full architectural fix (single reconciler, `gateway_router_middlewares` join table, layer4 app) is deferred to Phase B per plan.

---

## 2. Findings → Hotfix Mapping

| Finding | Short title | Hotfix | File:Line | Status |
|---|---|---|---|---|
| **F-NET-01** | Empty-sync wipes config every 30s | Guard `len(mergedRules)==0` → no-op | `crossnode/ingress_sync.go:128-135`, `175-182` | FIXED (hotfix) |
| **F-NET-02** | Asymmetric merge (traffic wipes domains) | Fetch-merge-preserve + sub-resource `POST /config/apps/http/servers/gamepanel` | `caddy_proxy.go:704-722`, `946-982`, `1211-1250` | FIXED (hotfix) |
| **F-NET-03** | Validate = apply + snapshot-after | Snapshot BEFORE `validateConfig`; dry-run via `POST /adapt` fallback + `must-revalidate` | `caddy_proxy.go:704-713`, `1086-1155`, `514:580-592` | FIXED (hotfix) |
| **F-NET-04** | Fictional `rate_limit`/`circuit_breaker` bricks updates | Gate behind `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` (default OFF) | `caddy_proxy.go:1058-1082`, `1084-1155` | FIXED (hotfix) |
| **F-NET-06** | Grouped-route miss (withdraw misses non-first) | Rebuild-minus-withdrawn group-aware via cached groups | `caddy_proxy.go:26-27`, `904-919`, `56-250` ; `traefik_proxy.go:22-30`, `246-265`, `294-430` | FIXED (hotfix) |
| **F-NET-07** | TCP mis-render (HTTP host+path for tcp) | Protocol branch: reject tcp/udp with 400 or skip HTTP (layer4 stub comment) | `caddy_proxy.go:690-702`, `1030-1045`, `service.go:383-393`; `handlers_trafficmanager.go:23-32,42-57` | FIXED (hotfix) |

*F-NET-05 (all-policies-all-routes) is not in this slice's 6 but was partially mitigated via F-NET-04 gating; full fix requires join table (Phase B).*

---

## 3. Implementation Details (no schema)

### 3.1 F-NET-01 Empty-sync guard — `crossnode/ingress_sync.go:111`

**Before:**
```go
groups := GroupRulesByRoute(rules)
var mergedRules []*trafficmanager.RoutingRule
for key, grp := range groups { ... }
if err := is.adapter.UpdateRoutes(ctx, mergedRules, policies); err != nil { ... }
```
`SetRules`/`UpsertRule` have zero non-test callers; Sync runs every 30s (`cmd/api/main.go:994`) with empty `is.rules` → `mergedRules=nil` → `updateRoutesAtomic` builds bare `gamepanel` server and `POST /config/` replaces whole Caddy config, wiping `gamepanel-domains` every 30s.

**After (`ingress_sync.go:128-135`, `175-182`):**
```go
if len(rules) == 0 {
    slog.Info("ingress sync skipping empty rule set — no-op to protect gateway config")
    return nil
}
...
if len(mergedRules) == 0 {
    slog.Info("ingress sync no healthy backends — skipping gateway update to avoid wipe", "groups", len(groups))
    return nil
}
```
Second guard handles case where rules exist but all backends unhealthy → groups filtered to zero. No `UpdateRoutes` call, no event publish, no `lastSync` bump. Idempotent no-op.

**Blast radius:** periodic total outage of domain routes + TLS every 30s → eliminated. If synchronizer is later populated from `trafficmanager+domains`, guard still safe (only skips empty).

### 3.2 F-NET-02 Asymmetric merge — `caddy_proxy.go:673-720`, `1211-1250`

**Before (`caddy_proxy.go:707-720`, `1026-1052`):**
```go
routes := p.buildRoutes(rules, policies)
serverConfig := p.buildServerConfig(routes, policies) // only gamepanel
validate(...)
snapshot = getRunningConfig // after validate
applyConfig(serverConfig) // POST /config/ full replace
```
`UpdateDomainRoutes` (`caddy_proxy.go:523-592`) correctly merges:
```go
rawExisting := getRunningConfig(...)
cfg["apps"]["http"]["servers"]["gamepanel-domains"] = ...
validate(cfg)
apply(cfg) // full but merged, preserves gamepanel
```
Inverse path full-replaces, wiping domains+TLS. Both servers listen `:80` (`caddy_proxy.go:567,713`) → nondeterministic.

**After (`caddy_proxy.go:715-759`, `946-982`):**
- Fetch `previousConfig` BEFORE validate (also fixes F-NET-03).
- Unmarshal to `cfg map[string]any`, ensure `apps.http.servers` exists.
- `servers["gamepanel"] = {listen:[":80",":443"], routes: buildPolicyRoutes(...)}` — preserves sibling servers.
- Validate whole merged `cfg`.
- Apply via sub-resource: `POST /config/apps/http/servers/gamepanel` with `serverPayload` (`caddy_proxy.go:1211-1230`). Fallback to `POST /config/` full merged if sub-resource 404 (compat with tests/mocks).
- Mirror fix in `UpdateDomainRoutes` (`caddy_proxy.go:580-592`): reuse `rawExisting` as `previousConfig` (snapshot before validate), use `applyDomainServerConfig` (`caddy_proxy.go:1232-1250`) → `POST /config/apps/http/servers/gamepanel-domains`.

**Result:** `ApplyRoutes`/`SyncRoutes` no longer destroys `gamepanel-domains`; domain sync no longer destroyed by traffic sync. Last-writer-wins still exists (two writers), but data loss window closed. Phase B will collapse to single reconciler.

### 3.3 F-NET-03 Validate = apply + snapshot-after — `caddy_proxy.go:980-1002`, `1086-1155`

**Before:**
```go
func validateConfig(...) { POST /load with must-revalidate }
func updateRoutesAtomic(...) {
  validate(body) // POST /load IS apply per caddyconfig/load.go:68-72
  previous = getRunningConfig // snapshots NEW bad config as lastValid
  apply(...)
}
```
Rollback restores bad generation; double-apply per update.

**After:**
- `updateRoutesAtomic` (`caddy_proxy.go:704-713`): snapshot BEFORE validate, `p.lastValidConfig = previousConfig`.
- `validateConfig` (`caddy_proxy.go:1086-1120`): attempt true dry-run via `POST /adapt?adapter=json` (`validateViaAdapt`), fallback to `POST /load` with `must-revalidate` for compat (tests mock `/load`). `isAdaptNotSupported` checks 404/405.
- `UpdateDomainRoutes` similarly snapshots before (`rawExisting` → `previousConfig`).

**Reference violated:** Caddy `/load` replaces config even with `must-revalidate` (`caddy.go:115-142`); true dry-run is `/adapt` (`load.go:137-175`). Hotfix makes rollback fictitious→real; P0 safety restored without changing callers.

### 3.4 F-NET-04 Fictional handlers — `caddy_proxy.go:795-875`, `1058-1135`

**Before (`caddy_proxy.go:795-875`):**
```go
if policy.RateLimit > 0 { handles = append(handles, {"handler":"rate_limit", ...}) }
if policy.CircuitBreaker { handles = append(handles, {"handler":"circuit_breaker", ...}) }
```
No such modules in `modules/caddyhttp/*`; CB is `http.reverse_proxy.circuit_breakers` (`reverseproxy.go:109`). Must-revalidate dry-run fails → every `UpdateRoutes` with any policy errors out, bricking gateway. Tests assert broken rendering (`caddy_proxy_test.go:280-314,547-615`).

**After:**
```go
func experimentalHandlersEnabled() bool {
  v := strings.ToLower(strings.TrimSpace(os.Getenv("CADDY_ENABLE_EXPERIMENTAL_HANDLERS")))
  return v=="true"||v=="1"||v=="yes"||v=="on"
}
...
if policy.RateLimit > 0 {
  if experimentalHandlersEnabled() { emit } else { slog.Warn("rate_limit disabled...", "policy", policy.ID) }
}
if policy.CircuitBreaker {
  if experimentalHandlersEnabled() { emit } else { slog.Warn(...) }
}
```
Default OFF → invalid handlers never emitted, validation passes. Operators opting into custom Caddy builds can set `CADDY_ENABLE_EXPERIMENTAL_HANDLERS=true` to re-enable.

**Tests:** `caddy_proxy_test.go:280`, `547`, `584` updated to `t.Setenv("CADDY_ENABLE_EXPERIMENTAL_HANDLERS","true")` so they still verify rendering when flag on; when flag off, `TestNoPolicies_NoMiddleware` continues to pass. Full suite PASS (`go test ./internal/services/trafficmanager` OK).

**Overload in Traefik:** Traefik equivalents are valid (`traefik_proxy.go:907-958`) and untouched; they correctly use `RateLimit`, `CircuitBreaker` middlewares.

### 3.5 F-NET-06 Grouped-route removal — `caddy_proxy.go:60-250`, `traefik_proxy.go:294-430`

**Before (`caddy_proxy.go:60-74`):**
```go
for _, id := range ruleIDs {
  DELETE /id/gamepanel-<id>
}
```
Grouped routes are `@id gamepanel-<firstRuleID>-group` (`caddy_proxy.go:949-951`) and `gamepanel-<primary.ID>` (`traefik_proxy.go:763-764`). Withdrawing non-first member → DELETE misses, gateway keeps dead upstream until full rebuild. `CleanupStale` already handled `-group` suffix (`caddy_proxy.go:350-362`) but still per-first.

**After — Caddy (`caddy_proxy.go:26-27`, `904-919`, `56-250`):**
- Add cache: `cachedGroups []routeGroup`, `cachedRuleMap map[string]*RoutingRule` (`caddy_proxy.go:26-27`), populated in `updateRoutesAtomic` (`caddy_proxy.go:904-919`).
- `RemoveRoutes` (`caddy_proxy.go:56-180`):
  - If `cachedGroups` non-empty, build `withdrawnSet`, filter groups: `kept = {r in grp.rules | r.ID not in withdrawnSet}`.
  - If `len(kept)==0` → whole group removed; if partial → `newGrp.rules=kept`.
  - Rebuild `routes` via `buildGroupedRoute` (skips tcp), fetch `rawExisting` to preserve other servers, set `servers["gamepanel"]`, `applyServerConfig` sub-resource, update cache (`cachedGroups=newGroups`, delete from `cachedRuleMap`).
  - Fallback (cache empty): try `DELETE /id/gamepanel-<id>` and `DELETE /id/gamepanel-<id>-group`, scan `presentRoutes` for logging, brute-force.

**After — Traefik (`traefik_proxy.go:22-30`, `246-265`, `294-430`):**
- Same cache (`cachedGroups []traefikRuleGroup`, `cachedRuleMap`).
- `UpdateRoutes` (`traefik_proxy.go:246-265`) caches `groupRules(rules)`.
- `RemoveRoutes` (`traefik_proxy.go:294-430`):
  - Group-aware filter via `withdrawnSet` and `cachedGroups`, building `keptGroups`.
  - Surgical patch: `cfg := loadRunningConfig()`; for each `origGrp` with `hasWithdrawn`, find `kept` by `domain|path|protocol`, if `kept==nil` → `delete(cfg.HTTP.Routers[...])`/`Services` (and `TCP` variants), else patch `svc.LoadBalancer.Servers` from `kept.rules` (preserving middlewares, TLS, EntryPoints).
  - Fallback per-id delete of `gamepanel-<id>`, `gamepanel-svc-<id>`, `gamepanel-tcp-<id>` etc, with warning for non-primary miss.

**Result:** `WithdrawNodeTargets` (`service.go:753-817`) and `ReconcileRoutes` (`service.go:900-951`) now correctly remove dead backends even when they are not group primary. Next full `UpdateRoutes` normalizes keys.

### 3.6 F-NET-07 TCP mis-render — `caddy_proxy.go:690-702`, `1030-1055`, `service.go:383-393`

**Before (`service.go:383-387`):**
```go
switch rule.Protocol {
case "", "http", "https", "tcp": // tcp admitted
}
```
`caddy_proxy.go:924-978` (`buildGroupedRoute`) always emits `{"host":..., "path":...}` + `reverse_proxy` regardless of `protocol`. Traefik (`traefik_proxy.go:758,854-905`) branches correctly but uses `HostSNI` which only matches TLS SNI → plain-TCP never matches. Caddy never emits `layer4` app.

**After:**
- `service.go:383-393` (`validateRoutingRule`): `case "", "http", "https": default: if protocol=="tcp"||"udp" { return fmt.Errorf("protocol %q not supported via HTTP gateway — use L4 load balancer...", protocol) }`. Admission now 400, not silent mis-render. UDP also rejected (was `unsupported` before, now explicit).
- `caddy_proxy.go:690-702` (`updateRoutesAtomic`): early reject for `tcp`/`udp` with `fmt.Errorf("protocol %q not supported via HTTP gateway (F-NET-07)...")`.
- `caddy_proxy.go:1030-1055` (`buildGroupedRoute`): protocol branch:
  ```go
  proto := strings.ToLower(strings.TrimSpace(grp.protocol))
  if proto=="tcp"||proto=="udp" {
    slog.Warn("F-NET-07: skipping tcp/udp group for HTTP server to avoid HTTP mis-render", ...)
    return nil
  }
  ```
  `buildRoutes` (`caddy_proxy.go:983-991`) skips `nil`. Layer4 stub comment left for Phase B:
  ```go
  // cfg["apps"]["layer4"] = { "servers": { "gamepanel-l4": { "listen": [":<port>"], "routes": [...] } } }
  // handler "proxy", match on remote IP/port. Not implemented in hotfix.
  ```
- `handlers_trafficmanager.go:23-32,42-57,107-112`: map `protocol not supported` errors to `400` via `strings.Contains(msg,"protocol")&&contains("not supported")` and `domainErrorStatus`.

**Traefik:** `traefik_proxy.go:758-764` already branches to `buildTCPGroupedRoute` (correct for TLS SNI). No rejection there; we log `tcp/udp rule via gateway — ensure TLS SNI or use L4 LB` in `UpdateRoutes` (`traefik_proxy.go:258-262`). Plain-TCP via Traefik still HostSNI-only; hotfix documents that L4 LB is canonical path (`loadbalancer/dataplane.go:115-270`).

**Blast radius:** L4 rules previously became unreachable HTTP routes → now receive clear 400, operator directed to `LOAD_BALANCER_ENABLED` path.

---

## 4. Files Modified (no schema)

1. `forge/api/internal/services/crossnode/ingress_sync.go:128-135,175-182` — dual guard (empty `rules` and empty `mergedRules`).
2. `forge/api/internal/services/trafficmanager/caddy_proxy.go:26-27,56-250,690-982,984-991,1030-1055,1086-1155,1211-1250` — sub-resource merge, snapshot-before, gater, protocol branch, group cache, `applyServerConfig`/`applyDomainServerConfig`, `validateViaAdapt`.
3. `forge/api/internal/services/trafficmanager/traefik_proxy.go:22-30,246-265,294-430` — group cache, surgical group-aware removal.
4. `forge/api/internal/services/trafficmanager/service.go:383-393` — tcp/udp admission rejection.
5. `forge/api/internal/http/handlers_trafficmanager.go:3-4,23-32,42-57,107-112` — 400 for protocol errors, 422 via `domainErrorStatus`.
6. `forge/api/internal/services/trafficmanager/caddy_proxy_test.go:280,547,584` — `t.Setenv("CADDY_ENABLE_EXPERIMENTAL_HANDLERS","true")` for 3 tests.

No migrations, no DB schema, no new tables.

---

## 5. Verification

**Static:** every `file:line` below was read before citing; `go vet ./forge/api/internal/store` OK; `go vet ./forge/api/internal/services/trafficmanager` OK after fix.

**Unit:**
```bash
go test ./internal/services/trafficmanager -count=1 -v   # PASS (all 30+ tests)
go test ./internal/services/crossnode -count=1 -v        # PASS (scenario7 etc)
```
- `TestAtomicReload_Success` → expects `POST /load` then `POST /config/`; now does `GET /config/` snapshot before, `POST /adapt` attempt (404 fallback), `POST /load`, `POST /config/apps/http/servers/gamepanel` → fallback to `POST /config/` → PASS with WARN.
- `TestMiddlewareRendering_RateLimit` / `CircuitBreaker` → now require env flag; set via `t.Setenv` → PASS.
- `TestRepro` (manual) for IP whitelist → now renders correctly after reverting `buildPolicyRoutes` → PASS.

**Logic:**
- Empty-sync: `GroupRulesByRoute([])` → `len(rules)==0` guard → `Sync` returns nil, no `UpdateRoutes` call (verified via log).
- Asymmetric merge: `updateRoutesAtomic` with 1 traffic rule + pre-existing `gamepanel-domains` route in mock `GET /config/` → after `POST /config/apps/http/servers/gamepanel`, `GET /config/` still contains `gamepanel-domains` (verified via mock).
- Snapshot-before: `previousConfig` captured before `validateConfig`; `validateConfig` fallback to `/load` still mutates but `lastValidConfig` is pre-mutation → restore correct.
- Gating: `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` unset → `buildPolicyHandles` returns no `rate_limit`/`circuit_breaker`, validation passes; set `true` → emits.
- Grouped removal: `cachedGroups` with 2 rules same domain/path, withdraw second → `RemoveRoutes(["r2"])` → `newGroups` keeps 1 rule, `POST /config/apps/http/servers/gamepanel` with 1 upstream → PASS.
- TCP: `CreateRoutingRule{Protocol:"tcp"}` → `service.go:386` returns `protocol "tcp" not supported...` → handler 400 → not rendered as HTTP.

**Negative:**
- `validateViaAdapt` with mock returning 404 correctly falls back to `/load` (isAdaptNotSupported).
- `applyServerConfig` with mock returning 404 correctly falls back to `applyConfig` (sub-resource fallback).

---

## 6. Risks & Deferred (Phase B)

- **Five-writer race remains:** `trafficmanager.SyncRoutes` (manual), `domains.syncCaddyRoutes`, `crossnode.Sync` (30s, now no-op when empty but still writer if populated), `loadbalancer` (separate), `proxy_domains` (no renderer). Phase B must collapse to single reconciler owning desired vs reported diff (Traefik `*dynamic.Configuration` or Caddy single-doc watcher) as per `audits/final-parity/subagent-07` §14.
- **F-NET-05 still leaks** if policies are used: `buildPolicyRoutes` still prepends all policy handles to all routes (deterministic order via map iteration random). Hotfix gates fictional handlers, but IP whitelist/blacklist still leak cross-tenant. Full fix: `gateway_router_middlewares` join table, deterministic order `rate-limit→blacklist→whitelist→CB→redirect`.
- **Traefik `/api/refresh` dead:** `traefik_proxy.go:1065-1084` still POSTs to nonexistent endpoint; file-watch is actual mechanism. Tests stub it; prod will always revert. Phase B: drop `reloadTraefik` or make advisory.
- **Layer4 incomplete:** Caddy `layer4` app and Traefik `ClientIP/CatchAllNoTLS` + `udp:` section not implemented; tcp rules rejected. If product needs game TCP, implement `apps.layer4.servers` or keep L4 LB as canonical.
- **HealthFilter producer-less:** `RecordSuccess/Failure` have zero callers (`health_filter.go:76-124`); `FilterHealthy` is pass-through. Fix in Phase B: feed from `ProbeTargets` + LB dataplane + `stale_reaper`.
- **CleanupStale still per-first:** `caddy_proxy.go:350-362` and `traefik_proxy.go:549-604` not fully group-aware; they will be fixed when they use `cachedGroups`.

**Rollback:** revert `CADDY_ENABLE_EXPERIMENTAL_HANDLERS` to `true` to restore old (broken) rendering if needed; otherwise no schema to rollback.

---

## 7. Handoff for Phase B (next)

Per `audits/FORGE_IMPLEMENTATION_PLAN.md` and `phase-04/synthesis` F-NET-01…10, the next hard work is:

1. **Single config authority:** promote `crossnode.RouteGroup` as sole grouping source, single `GatewayReconciler` that `GET /config/` once, merges `traffic_rules + proxy_domains + tls` and `POST /config/` or sub-resource once. Remove `IngressSynchronizer` or populate it from DB.
2. **Typed middleware references:** create `gateway_middlewares` + `gateway_router_middlewares` (Traefik `Middlewares[]string` shape), port `sanitizeDomains` checks, fix `traefik_proxy.go:797` backtick injection (`Path` → structured matcher).
3. **Cert delivery:** wire `gateway_adapter.SetCertificate` after `acme/service.go:355-363` and `dns/service.go` issuance; write `*.pem` files for Traefik `certFile/keyFile`, patch `apps.tls.certificates.load_files` for Caddy; instantiate or delete `CaddyTLSManager`.
4. **Health:** unify `ProbeTargets` (2s dial) + `dataplane` + `HealthFilter` threshold hysteresis; builders consult `healthStatus` before re-adding.
5. **L4:** decide: keep `loadbalancer` as L4 surface and keep gateway TCP rejected, or emit `layer4` app + proper Traefik `udp:`.

Until then, this hotfix stops the 30s wipe, the asymmetric wipe, the brick-on-policy, the dead-node leak, and the TCP mis-render with zero schema change.

---

## 8. Evidence Index (representative `file:line` inspected)

- Wiring: `forge/api/cmd/api/main.go:994` `ingressSync.Start(30*time.Second)`, `982` `NewCaddyReverseProxy`
- Caddy adapter: `caddy_proxy.go:44-49` struct, `673-705` `updateRoutesAtomic`, `707-720` `buildServerConfig`, `795-875` `buildPolicyHandles`, `894-922,924-978` grouping + `buildGroupedRoute`, `980-1052` `validateConfig/getRunningConfig/applyConfig/restoreConfig`, `1211-1250` sub-resource helpers
- Traefik adapter: `traefik_proxy.go:223-296`, `702-719` `buildConfig`, `725-753` `groupRules`, `758-852` `buildGroupedRoute`, `1065-1084` `reloadTraefik`
- Service: `service.go:24-54` models, `361-405` `validateRoutingRule`, `291-359,753-951,1001-1082` CRUD/withdraw/probe
- Crossnode: `ingress_sync.go:111-198,200-228`, `routegroup.go:32-97`, `health_filter.go:42-56,76-124`, `resolver.go:59-111`
- Handlers: `handlers_trafficmanager.go:15-67,107-112`
- Tests: `caddy_proxy_test.go:15-278,280-314,547-615`, `traefik_proxy_test.go:21-341`, `crossnode/scenario7_test.go`

---

*Report modified to `/Users/riyaz/project/gamepanel/audits/110-phase-03-impl/subagent-07-gateway-hotfix.md` per task.*
