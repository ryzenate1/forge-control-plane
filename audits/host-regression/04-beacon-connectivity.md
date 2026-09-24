# 04 — Beacon Connectivity

> Beacon = host daemon (`beacon/`), Forge = control plane (`forge/api`).  Citations against
> HEAD `ca06f74` unless noted.

## 4.1 Registration, token, and authentication

* **Node creation:** `forge/api` exposes `POST /api/v1/nodes` (`forge/api/internal/http/handlers_*`).
  `createNode` stores `nodes.base_url` and a daemon credential (id+secret) via
  `forge/api/internal/store/store_nodes.go` / `store_apphosting.go`.  The response returns
  the one-time `{node, token}` (`packages/sdk/src/client.ts:164-169` `CreateNodeResult`).
* **Credential storage:** `Store.GetNodeDaemonCredential(ctx, nodeID)` (used by
  `handlers_host.go:27` and `:43`).  Missing credential → `502 "node credential unavailable"`.
* **Beacon config:** `beacon/config/config.go:134-136` validates `token`/`tokenId` pair;
  `beacon/cmd/daemon/main.go:87-118` resolves `nodeToken` from `beaconConfig.Token`,
  `DAEMON_NODE_TOKEN`, `DAEMON_NODE_TOKEN_FILE` (`readSingleLineSecret`), or legacy `WINGS_TOKEN`.
* **Auth model:** Every Forge→Beacon request uses HMAC-style `Authorization: Bearer <token>`
  (`forge/api/internal/daemon/client.go: newRequest()` sets `Authorization`,
  `beacon/internal/auth/middleware.go` verifies).  `forge/api/internal/auth/remote_hmac.go`
  (stash adds `remote_hmac.go`, workdir has it) implements secondary remote HMAC for
  some routes.  `beacon/internal/auth/middleware_test.go` covers scope.
* **mTLS:** Optional defense-in-depth.  `beacon/cmd/daemon/main.go:388-403` (stash) adds
  `DAEMON_MTLS_CA_FILE` / `MTLS_CA_CERT` / `BEACON_MTLS_CA` handling; `forge/api/config/mtls.go`
  configures panel→beacon client CA.  When no CA is set, HMAC alone is sufficient. No Host
  failure is mTLS-related.
* **Result:** Authentication is not the Host regression. The credential path
  `Store.GetNodeDaemonCredential → daemon hostGet Authorization` is present in HEAD.

## 4.2 Transport: how Forge talks to Beacon for Host

* **API client construction:** `ForgeApiClient` (`packages/sdk/src/client.ts:264-420`) and the
  panel's `sharedApiClient()` (`forge/web/lib/api/http.ts:86-99`) both wrap `globalThis.fetch`
  with auth, CSRF, `credentials: include`, per-request `timeoutMs` (SDK default 30s), and
  idempotent-GET retries on 429/502/503/504 (`RETRYABLE_STATUSES`, `retryDelayMs`).
* **Daemon client (Forge → Beacon):** `forge/api/internal/daemon/client.go` creates an
  `http.Client` with `Timeout`, TLS, and mTLS options; `forge/api/internal/daemon/host.go:11`
  further bounds each Host call to `10*time.Second` via `context.WithTimeout`.
* **Beacon server:** `beacon/internal/server/server.go` builds an HTTPS (or plaintext loopback)
  `http.Server` at `DAEMON_ADDR` (`:9090` by default), with `Host` routes `GET /v1/host/{info,disk,memory,network,processes}`.
* **Host handlers (beacon):** `beacon/internal/server/handlers_host.go:45-130`
  — each is a pure `writeJSON(StatusOK, struct)` with no auth branching (auth is middleware).
  `handleHostInfo` assembles `HostInfo` from `os.Hostname`, `runtime.GOOS/ARCH`,
  `runtime.NumCPU`, `unix.Uname` (`kernelVersion`), and `cpuModelPlatform()`.

## 4.3 Heartbeat / status / "node offline"

* **Beacon heartbeat:** `beacon/cmd/daemon/main.go:696-790` `heartbeatLoop()` — ticker `30s`
  plus `0-5s` jitter, `remote.NodeHeartbeat` with `Version`, `ServerCount`, `LoadAverage`,
  `CPUPercent/Memory* /Disk* /Network*` (stash adds sysmetrics).  Sends via
  `client.SendNodeHeartbeat(ctx, nodeID, heartbeat)` (`beacon/internal/remote/client.go`).
* **API receipt:** `forge/api/internal/daemon/` and `forge/api/internal/http/handlers_*`
  plus `forge/api/internal/store/store_nodes.go` persist `last_seen`, `heartbeatState`.
  `AdminHealth` (`forge/web/components/admin/AdminHealth.tsx:230-260`) computes
  `healthyNodes = nodes.filter(n=>n.heartbeatState==="healthy")`, and
  `AdminNodes` (`AdminNodes.tsx`) displays status badge.
* **Host is NOT heartbeat-gated.**  `handlers_host.go:74-89` does not check heartbeat;
  it always attempts `GetHostInfo(ctx, target.NodeURL, target.NodeToken)`.  "Node offline"
  for Host therefore means **the Host proxy request failed**, not that heartbeat is stale.
  The two signals are independent:
  * `heartbeatState` → infrastructure overview (global)
  * `GET /host/info` 5xx → per-tab Host error (502/504)

  Collapsing them into a single "Node Offline" banner would be wrong — the UI correctly
  keeps them separate (`page.tsx:60` `formatError` maps 502 vs 504 vs 404).

## 4.4 Beacon channels

| Channel | Owner | Protocol | Host relevance |
|---|---|---|---|
| Host info/disk/memory/network/processes | API → Beacon (proxy) | HTTPS GET `/v1/host/*` via `daemon/host.go` | **Direct subject of audit** |
| Heartbeat | Beacon → API | HTTPS POST `remote.NodeHeartbeat` (client.go) | Not Host data, but explains "node status" vs "Host unreachable" distinction |
| Command channel (ops, console, files) | API ↔ Beacon | WS/HTTP per workload (`realtime.go`, `ws*`, `handlers_ws_*`) | Same proxy pattern; file Host via `host-files.ts` |
| Metrics channel | Beacon → API (heartbeat piggyback) | HTTPS POST heartbeat metrics | Separate from on-demand Host |
| Terminal | Browser ↔ Beacon via API WS ticket | WS `/api/v1/servers/{id}/ws/console` (`api.ts:921-938`) | Not Host |
| SFTP | Direct TCP + auth via Beacon | SFTP server (`beacon/internal/sftpserver`) | Not Host |

Forge never does Beacon operation over the terminal channel; Host is always API-proxied.
The frontend does **not** talk to Beacon directly for Host (unlike WS console which
uses a ticket to bridge to Beacon). This is intentional — only the API holds the node token.

## 4.5 Failure-to-report mapping

| Beacon failure | API mapping (`mapDaemonError`) | Frontend rendering |
|---|---|---|
| TCP dial timeout / no route | `url.Error{Timeout:true}` → `504 "Node unreachable — connection timed out"` | `formatError 504` → "Connection timeout — Beacon daemon did not respond in time" |
| Context deadline (10s hostCtx or 15s browser AbortSignal) | `context.DeadlineExceeded` → `504 "Node unreachable — request timed out"` | 504 line above |
| Connection refused / TLS failure | `&url.Error{Timeout:false}` → `502 "Node offline — Beacon daemon unreachable"` | `formatError 502` → "Node offline — Beacon daemon is unreachable" |
| Credential unavailable | `fiber 502 "node credential unavailable"` (before dial) | 502 path |
| Node not found / no nodes | `fiber 404 "node not found"` / "no nodes available" / "no reachable nodes" | `formatError 404` → "No node found — register a node first" |
| Beacon internal error (500) | `fmt.Errorf "host request failed with status %d"` → `502` with that text | raw message |
| API not configured / Daemon nil | `fiber 503 "daemon client and store are required"` | 503 → "Service unavailable — daemon proxy is not ready" |

All branches terminate in a visible state. HEAD has no infinite hang here.

## 4.6 What remains broken in Host vs what is healthy

* **Healthy (HEAD):** Beacon handlers exist, Forge→Beacon dial is bounded (10s),
  API error mapping distinguishes 504/502/404/503, heartbeat is independent and jittered,
  auth & mTLS paths are configured.
* **Broken (HEAD + workdir):** Frontend Host fetcher omits `?nodeId=` (host.ts:50, page.tsx:74),
  so the healthy proxy is called with the wrong target.  The fix is client-only
  (stash@{0} diff `be53ab7→f7c4bf8`); Beacon and ForgeHost plumbing are not implicated
  beyond being exercised with the wrong node.

## 4.7 Configuration pitfalls checked and cleared

* `API_BASE_URL` resolution (`http.ts:13-26`): runtime `window.__FORGE_CONFIG__`, then
  `NEXT_PUBLIC_API_URL` / `NEXT_PUBLIC_API_BASE_URL`, else `/api/v1` — not Host-specific,
  but Host inherits it.  Misconfigured env → general "API unreachable" (0/status 5xx),
  not per-node failure. Checked: no Host-specific env var.
* `forge/api/config/env.go:68` `DAEMON_NODE_TOKEN` consumed only by Beacon; not by Host handler.
* Stale `node.BaseURL` would cause 502/504 per-node — that would be an operator issue,
  not a regression; Host correctly surfaces it as 502/504 for the specific node when
  `?nodeId` is supplied.

