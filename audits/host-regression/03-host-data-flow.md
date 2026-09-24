# 03 — Host Data Flow

> Traces `AdminHost` page.tsx → React Query → API client → Fiber handler → Store → Daemon client → Beacon → response → render.  All file:line citations are against current HEAD `ca06f74` unless marked `stash@{0}`.

## 3.1 End-to-end chain

```
Host UI                          forge/web/app/admin/host/page.tsx:319-397  (AdminHost)
  ↓ nodeId state
React component                  page.tsx:71-106  InfoTab (and Disk/Memory/Network/Processes)
  ↓ useHostQuery(["host-info", nodeId], fetchFn, 30s)
React Query (TanStack)           page.tsx:32-49   useHostQuery()
  ↓ queryFn({signal}) → AbortSignal.timeout(15s) + AbortSignal.any
API client                       forge/web/lib/api/host.ts:50-67
  ↓ fetchJSON('/host/info', {signal})
HTTP layer                       forge/web/lib/api/http.ts:102-135  requestJSON() → ForgeApiClient.send()
  ↓ GET /api/v1/host/info[?nodeId=…]   (with credentials: include, CSRF if mutation)
Authentication                   forge/api/internal/http/auth.go, middleware_* — session cookie, requireRole("admin")
Authorization                    forge/api/internal/http/handlers_host.go:80  requireRole("admin")
Handler                          forge/api/internal/http/handlers_host.go:74-89  (GET /host/info)
  ↓ resolveNodeHostTarget(cfg, c.Query("nodeId"))
Service resolution               handlers_host.go:20-46  Store.GetNode / GetNodeDaemonCredential / ListNodes
  ↓ cfg.Daemon.GetHostInfo(ctx, target.NodeURL, target.NodeToken)
Daemon service (Forge → Beacon)  forge/api/internal/daemon/host.go:20-38  hostGet() → GET {BaseURL}/v1/host/info
  ↓ c.httpClient.Do(req)  (10s hostCtx)
Database                         forge/api/internal/store/store_nodes.go  (GetNode, GetNodeDaemonCredential, ListNodes)
  ↓ node record (id, name, BaseURL, token)
Beacon connectivity              beacon/internal/server/handlers_host.go:45-65  handleHostInfo
  ↓ net/http + unix.Statfs / runtime.NumCPU / os.Hostname / kernelVersion()
Heartbeat / status               beacon/cmd/daemon/main.go:696-790  heartbeatLoop (30s + jitter, sends NodeHeartbeat)
Response path                    Beacon JSON → daemon hostGet decode → Fiber c.JSON → browser JSON → React Query data
Frontend state                   page.tsx:79-85  isLoading/error/data/dataUpdatedAt branches
Rendered Host page               page.tsx:79-103  AdminLoadingState / AdminErrorState / EmptyState / StatRow grid
```

## 3.2 Per-step table

| Step | File | Function / route | Endpoint | Expected response | Actual (HEAD) | Failure behavior |
|---|---|---|---|---|---|---|
| Host UI shell | `forge/web/app/admin/host/page.tsx:319-397` | `AdminHost()` | — | Tabs + Card render | OK | — |
| nodeId state | `page.tsx:321` `useState("")` + `page.tsx:323-329` nodesQuery → `pickDefaultNode` | — | — | picks `status==="active"` else first node | OK | Empty set → "No nodes available" card (`page.tsx:357-365`) |
| Per-tab query | `page.tsx:71-75` `InfoTab({nodeId})` | — | — | queryKey `["host-info", nodeId]` | OK | But fetcher ignores `nodeId` |
| Query wrapper | `page.tsx:32-49` `useHostQuery()` | — | — | 15s timeout, retry 1, refetch 30s, stale 10s | OK | AbortSignal merged via `AbortSignal.any` |
| Host API client | `forge/web/lib/api/host.ts:50` `fetchHostInfo(init?)` | `GET /host/info[?nodeId=]` | `HostInfo` JSON | **BUG:** emits `/host/info` with **no `?nodeId`** (current) vs `stash@{0}` emits `/host/info?nodeId=…` | Backend falls back to first node; if that node has no BaseURL/credential → 404 "no reachable nodes"; if wrong node offline → 502. |
| HTTP transport | `forge/web/lib/api/http.ts:102-135` `requestJSON()` + `packages/sdk/src/client.ts:353-420` `execute()` | — | — | credentials include, CSRF on non-GET, timeout `timeoutMs 30s`, idempotent GET retries 3× on 502/503/504 with backoff | OK | RETRYABLE_STATUSES includes 502/503/504, so Host 502 may be retried twice before surfacing. |
| Auth | `forge/api/internal/http/auth.go` + `middleware_*` | — | — | session-cookie + `requireRole("admin")` | OK | 401 triggers `notifySessionExpired` event, not Host "offline". |
| Handler | `forge/api/internal/http/handlers_host.go:74-89` `GET /host/info` | `/api/v1/host/info` | `HostInfo` JSON | calls `resolveNodeHostTarget(cfg, c.Query("nodeId"))` then `cfg.Daemon.GetHostInfo(ctx, …)` | OK if `?nodeId` present; **without it** (`page.tsx` bug) uses fallback list. |
| Node resolution | `handlers_host.go:20-46` `resolveNodeHostTarget()` | — | `nodeHostTarget` | explicit `nodeID != ""` → single lookup + credential; else scan `ListNodes()` for first `BaseURL!=""` with credential | Fallback is nondeterministic (node list order) and masks missing-param bug. |
| Store | `forge/api/internal/store/store_nodes.go` `GetNode`, `GetNodeDaemonCredential`, `ListNodes` | — | node row + credential | — | OK |
| Daemon client | `forge/api/internal/daemon/host.go:20-38` `GetHostInfo()` | `GET {BaseURL}/v1/host/info` | `json.RawMessage` | wraps `ctx` in `10*time.Second` hostCtx, sets `Accept: application/json`, returns 2xx JSON | OK (fix from 4177276) |
| Beacon handler | `beacon/internal/server/handlers_host.go:45-65` `handleHostInfo()` | `GET /v1/host/info` | `HostInfo` | reads `os.Hostname`, `runtime.GOOS/ARCH`, `NumCPU`, `kernelVersion()`, `cpuModel()` | OK |
| Beacon disk/mem/net/proc | `beacon/.../handlers_host.go:67-130` | `/v1/host/disk` etc | `DiskPartition[]`, `MemoryInfo`, … | `unix.Statfs("/")`, `totalSystemMemoryMB()`, `netInterfacesPlatform()`, `processListPlatform()` | Disk only reports "/" partition (simplification). |
| Error mapping | `forge/api/.../handlers_host.go:11-28` `mapDaemonError()` | — | 504 vs 502 vs 502 | DeadlineExceeded/Canceled → 504, url.Timeout → 504, other url.Error → 502 | OK — lets UI distinguish timeout vs unreachable. |
| Frontend error UX | `page.tsx:60-69` `formatError()` | — | string | 504→"Connection timeout …", 502→"Node offline …", 404→"No node found …", 503→"Service unavailable …" | OK, but never reached for missing-nodeId's 404 if fallback picks a node. |
| Loading guard | `page.tsx:79` `if (isLoading && !data) return <AdminLoadingState…>` | — | — | shows loading only while no cached data | With `placeholderData: (prev)=>prev`, refetches keep old data and do not flash loading. |
| Workdir contaminant | `forge/web/app/admin/host/page.tsx:7` `OfflineBanner` import | — | — | unrelated — not committed to HEAD | Cosmetic only. |

## 3.3 Failure modes in current HEAD

* **Missing `?nodeId`** (`host.ts:50` HEAD, `page.tsx:74` HEAD): React Query key says
  `["host-info","abc123"]` but HTTP request is `GET /api/v1/host/info` with no query.
  Two consequences:
  1. **Wrong node** — backend picks `ListNodes()[0]` (implementation order, usually creation
     order or DB primary-key order), not the selected node. Console shows "Host Management"
     for node A while data comes from node B.
  2. **"Node offline" for healthy node** — if `ListNodes()[0]` happens to be a node whose
     Beacon is down, Host reports 502→"Node offline — Beacon daemon is unreachable"
     even though the *selected* node is healthy. Conversely a healthy first node masks
     an unhealthy selected node.

* **Staleness** — `queryKey` diverges from fetch identity, so switching nodeId from A→B
  may return cached `["host-info",A]` data instantly (`placeholderData`) while the
  network fetches for *no* node (again fallback), then cache for `["host-info",B]`
  is never populated correctly.

* **No infinite hang in isolation** — `useHostQuery`'s `AbortSignal.timeout(15000)` plus
  `daemon/host.go`'s `10*time.Second` timeout guarantees every Host fetch settles
  (504 or 502) within ~15-30s, even when Beacon is down. A true infinite load would
  require both to be removed — they are present in HEAD (fix of 4177276). The *reported*
  infinite-load symptom is therefore not a hanging promise but the `isLoading && !data`
  guard never clearing because the query is retried or the nodes list itself never loads
  (see §05).

## 3.4 Stash fix path (6bebe02)

With `stash@{0}`:

```
InfoTab({nodeId}) → useHostQuery(["host-info", nodeId], (init)=>fetchHostInfo(nodeId, init))
→ host.ts:59 fetchHostInfo(nodeId, init) → fetchJSON(`/host/info?nodeId=${nodeId}`, init)
→ http.ts requestJSON → Forge GET /api/v1/host/info?nodeId=… → auth → handler resolves exact node
→ daemon GetHostInfo(nodeId's URL/token) → Beacon handleHostInfo → JSON → render
```

This restores the intended chain and eliminates both failure modes above.

## 3.5 Cross-file dependencies that must remain in sync

* Adding `?nodeId=` requires both `host.ts` and every call site (`page.tsx` ×5 tabs);
  fixing one without the other is not sufficient (capture layer and composition layer).
* Backend already supports `?nodeId=` (`handlers_host.go:74` `c.Query("nodeId")`);
  no backend change is needed.
* `forge/web/lib/api/monitoring.ts` and `forge/web/components/monitoring/*` are not
  affected — they proxy through different routes (see 04).

