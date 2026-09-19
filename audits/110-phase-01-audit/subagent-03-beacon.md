# Subagent 03 — Beacon Daemon Audit (Phase 01 — Preparation)

**Scope:** `beacon/` runtime, server, metrics, SFTP, backup, transfer, capabilities  
**Date:** 2026-08-24  
**Agent:** 110-01-03 / 10  
**Mode:** read-only — no code modified  
**Citations:** `file:line` relative to repo root unless absolute noted

---

## 1. File Inventory (inspected)

### 1.1 Entrypoint / daemon wiring
| File | Lines | Purpose |
|------|-------|---------|
| `beacon/cmd/daemon/main.go` | 958 | Cobra entry, env/config load, `runtime.NewFactory` init, `buildBackupAdapter`, SFTP goroutine, panel sync, heartbeat loop, TLS, rate-limit, graceful shutdown |
| `beacon/cmd/daemon/main.go:144-185` | — | Runtime provider selection + fallback to `UnavailableRuntime` |
| `beacon/cmd/daemon/main.go:337-378` | — | SFTP bind-address resolution (audit F-12 fix) |
| `beacon/cmd/daemon/main.go:731-787` | — | `heartbeatLoop` + `runtimeHeartbeatStatus` |
| `beacon/go.mod:1-36` | — | Go 1.26, docker v28.5.2, k8s v0.36.2, containerd v2 |

### 1.2 Server / manager
| File | Lines | Purpose |
|------|-------|---------|
| `beacon/internal/server/server.go` | 3982 | `Server` struct, `NewServerWithBackup`, route table (~120 handlers), `health`/`ready`/`metrics`, `create`/`install`/`power`, `authenticate`, `recoverPanics` |
| `beacon/internal/server/server.go:674-733` | — | `metrics()` handler — **task pointer `:727`** in Prometheus gather/`expfmt` block |
| `beacon/internal/server/server.go:3473-3540` | — | `authenticate` — HMAC replay, streaming vs buffered auth, `MaxBytesReader` |
| `beacon/internal/server/manager.go` | 960 | `ServerManager`, `Reconstruction`, crash auto-restart, circuit breaker, `persistPowerState`, `HandlePower`, `StartEventWatcher` |
| `beacon/internal/server/server_helpers.go` | 63 | `decodeJSONBody`/`limitBody` — `MaxBytesReader` + `MaxBytesError` detection |
| `beacon/internal/server/hostfiles.go` | 527 | Host FS API — denylist vs allowlist, `hostAtomicWrite`, upload/download 100 MiB caps |
| `beacon/internal/server/diagnostics.go` | 227 | `RunConnectivityDiagnostics`, `ProbePanelHealth` — pinned DNS/dial, TLS, restricted IPs |
| `beacon/internal/server/capabilities.go` | 340 | `collectCapabilities`, `CapabilityReport`, `VersionCompatibility` |
| `beacon/internal/server/stats_collector.go` | 134 | `StatsCollector.Collect` — **uptime mislabel** candidate |
| `beacon/internal/server/handlers_host.go` | 164 | `HostInfo` — `Uptime: int64(time.Since(s.started).Seconds())` |

### 1.3 Runtime
| File | Lines | Purpose |
|------|-------|---------|
| `beacon/internal/runtime/runtime.go` | 184 | `Runtime` interface, `Provider*` constants, `Reconciler`, request types |
| `beacon/internal/runtime/factory.go` | 53 | `Factory.CreateRuntime` — **phantom provider check point** |
| `beacon/internal/runtime/docker.go` | 1135 | `DockerRuntime` — `Create:151`, `reconcile`, `ensureImage`, image pinning, network/ports, workload lock, installer container |
| `beacon/internal/runtime/docker.go:151-176` | — | **Task pointer `:151`** — `Create` idempotent vs `Reconcile` |
| `beacon/internal/runtime/podman.go` | 59 | `PodmanRuntime` embedding `DockerRuntime` |
| `beacon/internal/runtime/containerd.go` | 922 | Build-tag `containerd`, `lockedBuffer`, task IO |
| `beacon/internal/runtime/kubernetes.go` | 1055 | `KubernetesRuntime` — pod lifecycle, probes, exec |
| `beacon/internal/runtime/firecracker.go` | 954 | Build-tag `firecracker` — **shared RW rootfs finding** |
| `beacon/internal/runtime/provider_firecracker_enabled.go:1-7` | — | `createFirecrackerRuntime` wrapper |
| `beacon/internal/runtime/provider_firecracker_disabled.go:1-9` | — | Stub error when tag absent |
| `beacon/internal/runtime/unavailable.go` | 69 | `UnavailableRuntime` — mock mode guard |
| `beacon/internal/runtime/enhanced_stats.go` | 134 | `DecodeEnhancedStats`, `CollectEnhancedStats` — correct per-container `UptimeSeconds` from `state.StartedAt` |

### 1.4 Backup
| File | Lines | Purpose |
|------|-------|---------|
| `beacon/internal/backup/local.go` | 1026 | `LocalBackup` — **task pointer `:238` `Create`**, restore journal, `sameOrDescendant` guard |
| `beacon/internal/backup/local.go:238-413` | — | `Create` walk — `.pteroignore` merge, `zip.Create`, `validateArchive`/`extractArchive` |
| `beacon/internal/backup/s3.go` | 414 | `S3Backup` — staging via `LocalBackup`, `uploadToS3` retry, `downloadToStaging` 50 GiB cap, checksum via metadata `sha256` |
| `beacon/internal/backup/backup.go` | — | Adapter interface, `AdapterType` |
| `beacon/internal/backup/verification.go` | — | Checksum/verification helpers |

### 1.5 SFTP / Transfer / Remote / Metrics / System
| File | Lines | Purpose |
|------|-------|---------|
| `beacon/internal/sftpserver/server.go` | 804 | `Server.Run`, SSH config (curve25519, chacha20/aes-gcm), `authRequest` HMAC, `quotaWriter`, `handler` (FileGet/Put/Cmd/List) |
| `beacon/internal/transfer/protocol.go` | 860 | `Engine` — `Register`/`Authorize`/`PrepareSource`/`AppendDestination`/`RestoreDestination`/`Cancel`, `createSecureArchive`/`extractSecureArchive`, `atomic .tmp` + `Rename` |
| `beacon/internal/remote/client.go` | 359 | `Client` — HMAC + nonce + timestamp, `SetDefaultBeaconVersion`, `validatePanelEndpoint` (HTTPS or loopback), retry/redirect guards |
| `beacon/internal/remote/reconnect.go` | 324 | `ReconnectClient` — **blind `StateConnected` finding**, `probePanel` multi-candidate, jitter |
| `beacon/internal/remote/reconnect_test.go` | 121 | `StateConnected` invariant tests |
| `beacon/internal/remote/types.go` | 151 | `ServerStats.Uptime: uptime_ms` vs `NodeHeartbeat.Uptime: uptime_seconds` — **mislabel axis** |
| `beacon/internal/metrics/metrics.go` | 136 | `PrometheusCollector`, `CollectProcess` (`s.started` + `processCPUTimes`), `normalizeMetricPath` |
| `beacon/internal/metrics/process_cpu_unix.go` | — | `getrusage` user/system times |
| `beacon/internal/system/atomic.go` | 66 | `AtomicString`/`AtomicBool` |
| `beacon/internal/system/activity_dedup.go` | — | `ActivityDedup` (2 s window, 100 cap) used by SFTP |

### 1.6 Not present (negative inventory — relevant to phantom finding)
- `beacon/internal/runtime/lxc.go` — **absent**
- `beacon/internal/runtime/kvm.go` — **absent**
- `beacon/internal/runtime/lxc_test.go`, `kvm_test.go` — absent
- Grep `ProviderLXC`/`ProviderKVM` in `beacon/` — **zero hits** (`default.grep` returned only `forge/api/internal/runtime/*`)

---

## 2. Known Findings (with current disposition)

### F-01 — Phantom providers LXC / KVM — **OPEN / SEV-1 (contract drift)**
- **Task prompt:** “phantom providers LXC/KVM”
- **Evidence:**
  - API advertises and proxies `LXCProvider = "lxc"` and `KVMProvider = "kvm"` — `forge/api/internal/runtime/runtime.go:14-15`, `forge/api/internal/runtime/lxc.go:9-152`, `forge/api/internal/runtime/kvm.go:9-151`, registered `forge/api/cmd/api/main.go:398-399` (`multiRT.Register(gpruntime.LXCProvider, ...)`). Docs claim selectable runtimes (`docs/audits/SERVICES_UI_GAP.md:14`).
  - Beacon **does not implement** LXC/KVM. `beacon/internal/runtime/runtime.go:10-16` defines only `docker|containerd|podman|firecracker|kubernetes`. `beacon/internal/runtime/factory.go:19-32` switch has no `lxc`/`kvm` cases — hits `default:` → `fmt.Errorf("unsupported runtime provider: %s")` → daemon startup fails (or `UnavailableRuntime` in mock mode). Grep for `LXC|KVM|phantom` inside `beacon/` returned **no implementation files**.
  - `beacon/cmd/daemon/main.go:145` reads `DAEMON_RUNTIME_PROVIDER` with default `docker` but no env plumbing for `DAEMON_LXC_*`/`DAEMON_KVM_*` referenced in docs.
- **Impact:** Panel can assign `runtime_provider="lxc"|"kvm"`; heartbeat will report it (`beacon/cmd/daemon/main.go:737-765` `RuntimeProvider` field) but daemon rejects creation → server `Create` surfaces 503/400, install loops. UI chooser shows unavailable runtimes (Phase 27 phantom analog).
- **Status:** Open. Docs incorrectly mark FIXED; code never landed.
- **Required fix:** Either (a) remove API adapters + registration and hide chooser entries, or (b) implement `beacon/internal/runtime/{lxc,kvm}.go` behind no-tag (exec-based) matching docs. Do not half-ship.

### F-02 — Firecracker shared RW rootfs — **OPEN / SEV-1 (data corruption)**
- **Evidence:** `beacon/internal/runtime/firecracker.go:262-268`
  ```go
  "path_on_host":   r.config.RootfsImage,
  "is_root_device": true,
  "is_read_only":   false,
  ```
  Same `r.config.RootfsImage` is used for every VM in `configureMicroVM:249-292` and `Install:436-443`. No per-instance overlay/qcow2/delta, no `is_read_only:true` + overlay. All microVMs share one ext4 image RW → concurrent writes corrupt image; `is_read_only:false` on host path violates Firecracker journal isolation.
  - `Create:171-195` only tracks `instances` map — no rootfs copy. `Install` mounts `rootDir` via MMDS but reuses same rootfs.
- **Impact:** Multi-tenant node with >1 firecracker server can interleave filesystem ops → silent corruption, ext4 journal replay failure after crash, non-deterministic boot.
- **Required fix:** Per-VM COW: copy `RootfsImage` to `filepath.Join(r.config.SocketPath, vmID+".ext4")` with `O_EXCL`, or use `overlay`/`qcow2` backing file, set original `is_read_only:true` and overlay RW. Clean up on `Delete`/`killInstance`. Add `Tmpfs`/snapshot accounting. Existing `ValidateCreate` path not enough.

### F-03 — Unbounded `rawBody` vs 100 MiB `hostUploadLimit` — **OPEN / SEV-1 (memory-DoS, cap bypass)**
- **Beacon side (correct):** `beacon/internal/server/hostfiles.go:457-458`
  ```go
  const hostUploadLimit = 100 * 1024 * 1024
  r.Body = http.MaxBytesReader(w, r.Body, hostUploadLimit)
  ```
  plus `hostAtomicWrite:124` `LimitReader(limit+1)` → 413 on exceed. Same fix exists in `beacon/internal/server/server.go:3519` `maxSignedStreamingBodyBytes = maxUploadChunkBytes (8 MiB)` for chunked route.
- **API side (vulnerable):** `forge/api/internal/http/handlers_files.go:402-403`
  ```go
  rawBody := c.Request().Body()
  req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(rawBody))
  ```
  plus `handlers_files.go:408` `req.ContentLength = int64(len(rawBody))` and `SignedHeaders(..., rawBody)`. `c.Request().Body()` in Fiber buffers entire body into memory with **no `MaxBytesReader`** / limit. Attacker POSTing multi-GB to `/api/v1/nodes/:id/files/upload?path=...` loads full payload into API RAM, signs it, proxies it — bypasses beacon's `MaxBytesReader` back-pressure and causes OOM before beacon can reject. Content-Type check on `handlers_files.go:396-397` happens after buffering.
- **Required fix:** API must stream: `c.Request().BodyStream()` / `io.CopyN` with `http.MaxBytesReader` equivalent on Fiber context, enforce `100*MiB+1` with `io.LimitReader`, use `io.NopCloser`/`io.Pipe` and `Transfer-Encoding: chunked` or `X-Accel-Buffering: no`; sign streaming header with `nil` body or chunked HMAC as beacon already expects for streaming uploads (`server.go:3506-3510` empty-body signature). Remove `rawBody` buffering.

### F-04 — Daemon uptime mislabel / dual-unit confusion — **OPEN / SEV-2 (observability)**
- **Evidence chain:**
  - `beacon/internal/remote/types.go:97` `ServerStats.Uptime int64 json:"uptime_ms"` (milliseconds)
  - `beacon/internal/remote/types.go:112` `NodeHeartbeat.Uptime int64 json:"uptime_seconds"` (seconds)
  - `beacon/cmd/daemon/main.go:763` heartbeat sends `Uptime: int64(time.Since(startTime).Seconds())` → seconds, correct for `uptime_seconds`.
  - `beacon/internal/server/server.go:683` `game_panel_daemon_uptime_seconds` — label says seconds but value is `time.Since(process.StartTime).Seconds()` → correct.
  - **But** `beacon/internal/server/stats_collector.go:61-74`:
    ```go
    ss = ServerStats{Timestamp: time.Now(), UptimeSeconds: time.Since(sc.startTime).Seconds()}
    // else branch same: time.Since(sc.startTime) not container StartedAt
    ```
    `sc.startTime` is **daemon/collector start**, not per-server container uptime. Every `ServerStats` for every `serverID` reports **daemon uptime**, not `runtime.Inspect(...).StartedAt`.
  - Correct implementation exists in `beacon/internal/runtime/enhanced_stats.go:127-132`:
    ```go
    if !state.StartedAt.IsZero() { stats.UptimeSeconds = time.Since(state.StartedAt).Seconds() }
    ```
  - `beacon/internal/server/server.go:727-732` Prometheus scrape appends `game_panel_daemon_container_*` per-server stats via `s.runtime.Stats(ctx, id)` — that path uses Docker's `StartedAt` inside runtime, so scrape mixes two uptime sources (daemon vs container) without clear HELP distinction. Dashboard `infra/grafana/.../gamepanel-overview.json:211` graphs `game_panel_daemon_uptime_seconds` as daemon uptime — correct — but API-exposed `ServerStats` confuses consumers that poll `/servers/:id/stats`.
  - `beacon/internal/server/handlers_host.go:65` `HostInfo.Uptime: int64(time.Since(s.started).Seconds())` — daemon uptime labelled as host uptime (misleading; true host uptime comes from `unix.Sysinfo`/`procfs`, not daemon `started`).
- **Impact:** Alerts and autoscaling that use `/servers/:id/stats` `uptime_seconds` get daemon restart age, not container age; host overview shows daemon uptime as machine uptime; cross-checks between API `store_capabilities.UptimeSeconds` and daemon heartbeat drift.
- **Required fix:** `stats_collector.go:61-74` → query `s.runtime.Inspect(ctx, serverID).StartedAt`; fallback to `0` if not running; keep daemon `started` for `NodeHeartbeat`/`metrics` only. Rename `HostInfo.Uptime` to `DaemonUptime` or populate from `hostUptime()` (sysinfo). Document unit in HELP: `// seconds, derived from container StartedAt`.

### F-05 — Reconnect `StateConnected` blind promotion — **PARTIALLY FIXED / SEV-2 (availability lie)**
- **Task prompt:** “reconnect blind `StateConnected`”
- **Current state:** Prior blind `atomic.StoreInt32(StateConnected)` on `Start()` removed. Now `beacon/internal/remote/reconnect.go:112` starts at `StateConnecting`, `RecordHeartbeatSuccess:262-273` is documented as “only path that makes a node appear connected.” `RecordHeartbeatFailure:277-283` demotes to `Reconnecting`. `doReconnect:157-201` probes before `StateConnected`.
- **Remaining blind/latent issues:**
  - `reconnect.go:128-143` `last.IsZero()` branch:
    ```go
    if last.IsZero() {
        if ConnState(atomic.LoadInt32(&rc.state)) == StateConnecting && time.Since(time.Now().Add(-rc.offlineTimeout)) >= 0 { }
        continue
    }
    ```
    No-op `if` body — prolonged `Connecting` without success **never transitions** to `Failed`/`Reconnecting`; never increments `attempts`. Node appears `Connecting` forever if first heartbeat never arrives (e.g., token rejected). `Attempts==0` persists; offline detection suppressed.
  - `probePanel:207-257` mutates `lastHb = time.Now()` on **probe success** (`reconnect.go:186-189`) — a lightweight `/health` 2xx without authenticated heartbeat can mark node `Connected`, mixing liveness probe with authenticated heartbeat success. Separates two success signals into same `lastHb`.
  - `RecordHeartbeatSuccess:269-272` blindly does `atomic.StoreInt32(StateConnected)` **without** re-checking current `state` is `Connecting|Reconnecting` — concurrent `Stop()` can be overwritten to `Connected` after `stopCh` closed; `RunConnectivityDiagnostics` not consulted.
  - `State()` is `atomic.LoadInt32` without happens-before `lastHb` lock → racy stats.
- **Required fix:** Replace `last.IsZero()` no-op with `if time.Since(rc.startTime) > offlineTimeout → StateFailed, attempts++`. Remove `lastHb = now` from `doReconnect` probe path; let `probePanel` return bool; only `RecordHeartbeatSuccess` writes `lastHb`. Guard `RecordHeartbeatSuccess` with `select { case <-rc.stopCh: return }` and require `old == Connecting||Reconnecting`. Initialize `startTime time.Time` on `Start`.

### F-06 — `docker.go:151` Create idempotent vs drift — **OPEN / SEV-2 (config drift)**
- **Evidence:** `beacon/internal/runtime/docker.go:151-166`
  ```go
  func (r *DockerRuntime) Create(ctx context.Context, req CreateRequest) error {
      lock := r.workloadLock(req.ServerID); lock.Lock(); defer lock.Unlock()
      existing, err := r.Inspect(ctx, req.ServerID); ...
      if existing.Exists { return nil }
      return r.reconcile(ctx, req)
  }
  ```
  `Create` **no-ops** if any container exists, regardless of `createRequestHash` mismatch. `Reconcile:171-249` checks hash (`configHashLabel`) and recreates if changed. Panel boot path calls `Create` (from `server.go:735-842` `create` handler) for new servers, and `Reconcile` only via `server.go:973-1004` `syncConfiguration`/runtime config path. If `Create` arrives after config change but before `syncConfiguration`, container retains stale config hash — ports/mounts/env/RAM drift silently.
- **Secondary:** `reconcile:198-215` removes container while `restartAfterCreate` logic races with concurrent `WatchEvents` — `HandleContainerEvent` may emit `die` → auto-restart cycle mid-reconcile; `workloadLock` keyed by `sha256(serverID)[0] % 64` yields 64-way sharding → 1/64 collision → unrelated servers block each other.
- **Required fix:** `Create` must delegate to `Reconcile` or call `createRequestHash` before short-circuit; if hash differs → `return r.reconcile`. Add `Reconcile` fast-path idempotency already present covers Create; no need for separate Exists short-circuit. Increase `workloadLocks` to `sync.Map[string]*sync.Mutex` per-server or 256.

### F-07 — Backup `local.go:238` Create hardening — **MOSTLY FIXED / SEV-2 lingering**
- **Evidence:** `beacon/internal/backup/local.go:238-413` `Create` now:
  - canonicalizes `canonicalRoot`/`canonicalDirectory`, guards `sameOrDescendant(l.backupRoot, canonicalRoot)` (`local.go:250`), uses `rootfs.New` for TOCTOU, respects `.pteroignore` (`259-271`), zip `Store` for dirs, `isRegular`+`SameFile` check (`349-360`), per-file `limitReader`-style via `copyWithContext`.
- **Remaining:** No per-file size cap inside zip (symlink already rejected; but sparse/large files can exhaust staging disk — `S3Backup.downloadToStaging:330-333` added `ensureStagingDiskSpace` for S3 path, but local `Create` has no equivalent `diskFreeFn` check before walk). `ignored []string` appended with `pteroignore` patterns but `IgnoredFiles` stored in `localMetadata:406` includes raw `patterns` (from both sources) — may leak absolute patterns if caller passes them.
- **Required fix:** `local.go:273` call `ensureStagingDiskSpace`/`diskFreeFn` before walk; add per-file `limitReader` of `int64(header.UncompressedSize64)` already in `downloadextract.go:246` pattern — mirror for local `Create`.

### F-08 — Server `server.go:727` metrics exposition — **FIXED with nit**
- **Evidence:** `beacon/internal/server/server.go:722-732`
  ```go
  families, err := prometheus.DefaultGatherer.Gather()
  encoder := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
  for _, family := range families { encoder.Encode(family) }
  ```
  Prior duplicate HELP bug fixed. Remaining nit: per-server loop `server.go:698-719` prints HELP/TYPE lines **per iteration** (per `serverID`) → repeated HELP blocks; technically valid Prometheus exposition but noisy and duplicates HELP for each label. `metrics.go:58-60` `MustRegister` guarded by `sync.Once` but multiple `NewPrometheusCollector()` calls share default registry → second collector's vectors are orphan (not re-registered) but still used locally — unexported. No `MaxHeaderBytes` clamp for `/metrics` scrape from loopback exempt path (`server.go:3475-3488` bypasses token but still allows unbounded `ServerIDs()` loop over many servers → 15 s scrape timeout via `server.go:697` `WithTimeout 5s`.
- **Required fix:** Hoist HELP/TYPE out of per-server loop; use per-registry not default for collectors; bound scrape to `List` pagination or `TopN` for large fleets.

### F-09 — `factory.go` runtime selection — **OPEN / SEV-3**
- `beacon/internal/runtime/factory.go:19-32` switch lacks `lxc`/`kvm` cases; error message `unsupported runtime provider: %s` is correct but **panel validation**. Should enumerate available in error (call `AvailableProviders()`). Env `DAEMON_RUNTIME_PROVIDER` case-sensitive (`Provider` constants lowercase) — mismatch if user sends `Docker`. `AvailableProviders` includes `firecracker` even when built without tag (`provider_firecracker_disabled.go` runtime unavailable at runtime) → advertise unavailable provider via `/api/capabilities` (`capabilities.go:138-142` `ComposeEnabled = true` unconditionally, but runtime list should filter by build tags).
- **Fix:** Filter `AvailableProviders` by build tags; normalize `strings.ToLower(strings.TrimSpace(f.config.Provider))`.

### F-10 — Capabilities delta / version gate — **OPEN / SEV-3**
- `beacon/internal/server/capabilities.go:133-145` `stackCount` always 0 (never populated from `composeStacks`). `RuntimeProvider` always `""` (`capabilities.go:97-100` sets `runtimeStatus` but not `runtimeProvider` string from `s.runtime.Provider()`). `handleGetCapabilitiesDelta:220-226` returns **full** report not delta → name lies; API expects delta fields `added/removed/changed`. `compareReleaseVersions` ignores `+build`/`-pre`.
- `beacon/internal/server/server.go:586-591` `SetVersion` trims but allows `beacon-dev` default — heartbeat `Version` gate on API may reject (`remote/client.go:269` `X-Beacon-Version`).

---

## 3. Files to Touch (beacon tree only — no code modified in this audit)

| Priority | File | Change type | Reason |
|----------|------|-------------|--------|
| P0 | `forge/api/internal/http/handlers_files.go:402-418` | **API streaming** (outside beacon but blocks beacon cap verification) | Remove `rawBody` buffering; stream with `BodyStream` + `MaxBytesReader` |
| P0 | `beacon/internal/runtime/firecracker.go:104-280` | Bugfix | Per-VM rootfs COW; `is_read_only:true` host base |
| P0 | `beacon/internal/runtime/factory.go:19-32` | Decision | Add `lxc`/`kvm` stubs or keep error but stop API registration (`forge/api/cmd/api/main.go:398-399`) |
| P0 | `beacon/internal/server/hostfiles.go:457-458` | Already fixed | Verify companion API fix ships together |
| P1 | `beacon/internal/server/stats_collector.go:61-74` | Bugfix | Use `Inspect.StartedAt` for `UptimeSeconds` |
| P1 | `beacon/internal/server/handlers_host.go:65` | Rename/fix | `Uptime` → `DaemonUptime` or true host uptime |
| P1 | `beacon/internal/remote/reconnect.go:128-143,186-192,262-283` | Bugfix | `startTime`, no-op branch, probe vs heartbeat separation, stop guard |
| P1 | `beacon/internal/runtime/docker.go:151-166` | Bugfix | `Create` hash-check or delegate to `Reconcile` |
| P2 | `beacon/internal/backup/local.go:273,250` | Hardening | `ensureStagingDiskSpace` before walk; pattern sanitization |
| P2 | `beacon/internal/server/server.go:698-732,3475-3488` | Perf/correctness | Hoist HELP, bound scrape, registry per-collector |
| P2 | `beacon/internal/server/capabilities.go:97-176,220-226` | Correctness | Populate `RuntimeProvider`, `StackCount`, true delta |
| P2 | `beacon/internal/runtime/factory.go:45-52` | Correctness | Lowercase/normalize, filter by build tags |
| P2 | `beacon/internal/server/diagnostics.go:97-98` | Already uses `/api/v1/health` — keep pinned | Ensure `ProbePanelHealth` uses same path as `reconnect.probePanel` candidate list (`reconnect.go:219`) |

### Files that must NOT be touched in this phase (explicitly out of scope)
- `beacon/internal/sftpserver/server.go` — host-key passphrase flow `sftpHostKeyPassphrase` (`daemon/main.go:544-573`) and rate limiter are adequate; no change.
- `beacon/internal/transfer/protocol.go` — archive/extract integrity (context-aware `copyWithContext`, `AtomicWriteExact`, `checksumFile`) is sound; no change.
- `beacon/internal/system/*` — `AtomicString`/`ActivityDedup` correct; no change.

---

## 4. Beacon Changes Needed (diff-level checklist)

### 4.1 Restore phantom-contract correctness
- **Option A — remove phantom (recommended for Phase 01):**
  - Delete `forge/api/internal/runtime/lxc.go`, `kvm.go`.
  - Remove `forge/api/cmd/api/main.go:398-399` registrations.
  - Hide/disable LXC/KVM entries in `web/` chooser and validation (`validateCreateRequest` runtime allowlist).
  - `beacon/internal/runtime/factory.go` — no change; add comment `// lxc/kvm intentionally unsupported in Phase 01`.
- **Option B — implement beacon LXC/KVM:** add `beacon/internal/runtime/{lxc,kvm}.go` (no build tag, exec-based `lxc-*`/`virsh`), `RuntimeConfig.LXC/KVM`, `DAEMON_LXC_*`/`DAEMON_KVM_*` envs in `cmd/daemon/main.go:148-171`, register in `factory.go`. Requires `init`/`cgroup` handling; not Phase 01.

### 4.2 Fix Firecracker RW aliasing
```go
// beacon/internal/runtime/firecracker.go:configureMicroVM
overlaid := filepath.Join(filepath.Dir(r.config.RootfsImage), vmID+".overlay.ext4")
copyFileWithLimit(r.config.RootfsImage, overlaid)
"path_on_host": overlaid,
"is_read_only": false, // per-VM
// original remains is_read_only:true backing or not mounted at all
// on Delete: os.Remove(overlaid) + socket + state
```

### 4.3 Make `Create` config-authoritative
```go
// beacon/internal/runtime/docker.go:151
func (r *DockerRuntime) Create(ctx context.Context, req CreateRequest) error {
    // remove Exists fast-exit; fall through to reconcile hash check
    return r.reconcile(ctx, req)
}
```

### 4.4 Fix uptime sources
```go
// beacon/internal/server/stats_collector.go:56
state, _ := sc.runtime.Inspect(ctx, serverID)
if !state.StartedAt.IsZero() { ss.UptimeSeconds = time.Since(state.StartedAt).Seconds() }
else { ss.UptimeSeconds = 0 }
// remove sc.startTime usage for per-server uptime
```

### 4.5 Fix reconnect state machine
- Add `startTime time.Time` to `ReconnectClient`, set in `Start` under lock.
- `run:128-143`:
  ```go
  if last.IsZero() && time.Since(rc.startTime) > rc.offlineTimeout { rc.setState(StateFailed); atomic.AddInt64(&rc.attempts,1); }
  ```
- `doReconnect:188` remove `rc.lastHb = now`; let caller `RecordHeartbeatSuccess` own `lastHb`.
- `RecordHeartbeatSuccess`: check `stopCh` before CAS; only `CAS(Connecting|Reconnecting → Connected)`.

### 4.6 Harden backup create (minor)
- `local.go:272-273` call `ensureStagingDiskSpace` with estimated `rootfs.Usage()`.

### 4.7 Capabilities polish
- `capabilities.go:97-100` set `runtimeProvider = s.runtime.Provider()` when `s.runtime != nil`.
- `handleGetCapabilitiesDelta` compute true `CapabilityDelta` (reuse `computeCapabilityDeltaFromExternal`).

---

## 5. Rollout Plan (beacon vs API order — explicit)

**Principle:** Beacon is stateful (containers, volumes, mounts); API is stateless proxy. Beacon rolls via rolling restart per node; API rolls as deployment. Beacon has panel `validatePanelOnboarding` gate; API has heartbeat `X-Beacon-Version` compatibility (`remote/client.go:269`).

| Step | What | Beac. | API | Order note |
|------|------|-------|-----|------------|
| 0 | **Tag & freeze** `beacon-dev` → `vX.Y.Z` (`Server.SetVersion`) | ✓ | — | Version must be set before any heartbeat-gated changes |
| 1 | **API hotfix — `rawBody` streaming** `forge/api/internal/http/handlers_files.go:402` | — | **FIRST** | API reads unbounded before beacon can enforce; beacon already capped. Ship API first so beacon cap is actually hit, not bypassed via proxy buffering. No beacon dep. |
| 2 | **API hotfix — phantom providers hide** (`lxc.go`/`kvm.go` delete or feature-flag off) | — | **FIRST** | Prevents panel assigning `lxc`/`kvm` to nodes that will reject. Beacon error string unchanged; safe to ship API before beacon decision. |
| 3 | **Beacon — Firecracker per-VM overlay** | **SECOND** (after steps 1-2) | — | No API contract change; rolling node restart. Keep single node canary; verify `firecracker.go:262` `is_read_only` change does not break existing single-VM nodes (backward compat: fallback to copy if overlay exists). |
| 4 | **Beacon — uptime + reconnect fixes** (`stats_collector.go`, `reconnect.go`) | **SECOND** | — | Observable-only; no API schema change. Deploy beacon before API consumes new `uptime` semantics if API later graphs container uptime. Safe either order, but beacon first gives correct data on next scrape. |
| 5 | **Beacon — `docker.Create` hash delegation** | **SECOND** | — | Change is idempotent-to-reconcile; rolling safe. Existing containers get recreated only if hash changed — canary one node, watch `WatchEvents`. |
| 6 | **Joint — capabilities true delta** | beacon emiss., API consumpt. | — | Ship beacon `handleGetCapabilitiesDelta` with API store that expects `delta` fields; keep legacy `GET /api/capabilities` (full report) until API delta consumer migrates. Feature-flag delta route. |
| 7 | **Decision point — LXC/KVM implement or keep hidden** | If B, beacon then API; if A, nothing more | — | **Beacon must be rolled before API re-advertises LXC/KVM** (if B). Reverse would reintroduce phantom. |

**Why beacon before API for LXC/KVM (if implementing):** Beacon `Factory.CreateRuntime` is gate; API `NewLXCAdapter` will succeed but daemon `Create` returns `unsupported runtime provider` → user-visible 500. Beacon must pass `Ping` health check first.

**Why API before beacon for `rawBody` & phantom hide:** Limits and validation at proxy ingress must be stricter than downstream; beacon caps cannot defend against upstream buffering OOM. Phantom hide is UI/API truth, no beacon code needed to stop offering bad options.

**Rollback affinities:** API `handlers_files.go` streaming change is internal (no schema); rollback safe. Firecracker overlay change is **not** rollback-safe without migration (VMs created with overlay expected on downgrade). Guard with `if _, err := os.Stat(overlaid); os.IsNotExist` fallback to base `is_read_only:false` for downgraded beacon.

---

## 6. Risks & Mitigations

| Risk | Trigger | Blast radius | Mitigation |
|------|---------|--------------|------------|
| **OOB write in Firecracker overlay path** | `filepath.Join(SocketPath, vmID+".overlay.ext4")` with malicious `serverID` containing `/`/`..` | Host FS escape | Reuse `containerName(serverID)` (`docker.go:714-716` → `mgp-<id>`), which already validates via `validContainerID:990-1001`; add same guard to firecracker `Create`. |
| **Container churn on `docker.Create` change** | Existing nodes have stale hash containers; new `Create→Reconcile` recreates them on next panel sync | Rolling restart of all game servers | Hash check is no-op if `configHashLabel == hash`; churn only if drift existed (desired). Throttle `Reconcile` via jittered startup, same as `syncServersFromPanel:576-637` already `errors.Join`. |
| **`reconnect` `StateFailed` flapping** | API health 2xx without auth → probe success but heartbeat still failing | Misleading `Failed`→`Connected` oscillation | Keep `probePanel` and heartbeat `lastHb` separated (Section 4.5); require `RecordHeartbeatSuccess` for `lastHb`. |
| **Metrics scrape storm** | `ServerIDs():223-235` enumerates all servers; per-server `Stats(5s timeout)` serial | `/metrics` timeout on large fleets (5 s × N) | Add `maxServersForMetrics = 100` or cache `Runtime.Stats` via `StatsCollector`; reduce timeout to 2 s per server; parallelize. |
| **S3 staging disk exhaustion** | `local.Create` walk writes zip under `backupRoot` without free check | Node disk fill | Add `diskFreeFn` check before walk (mirrors `S3Backup.ensureStagingDiskSpace:330`); alert on `hostUploadLimit` + zip headroom. |
| **Build-tag skew** | Default build excludes `firecracker`, `containerd` tags; `AvailableProviders` still advertises them | UI shows `firecracker` on non-firecracker binary → `unsupported runtime` | Filter `AvailableProviders` by `runtime.GOOS` + tag presence; API capability `StorageInfo` should not claim `TransferEnabled` if `transferProtocol == nil` (already guarded `capabilities.go:144`). |
| **Shared RW host volume semantics** | `hostfiles.go` `resolveHostPath` denylist overlaps with `firecracker` `SocketPath` under `/run/gamepanel` | Host FS API could touch firecracker socket dir | Ensure `hostFileDenylistPrefixes` includes `s.dataDir` already (`hostfiles.go:58`); add explicit excluder for `DAEMON_FIRECRACKER_SOCKET_PATH`. |
| **Signed-header replay window** | `server.go:3501` `5 min` skew + `65536` nonces | Burst of 65k distinct nonces within 5 min evicts live nonces → replay window | Acceptable; `nonceMu` LRU eviction only when full; worst-case legit rate far below cap (comment `server.go:3552`). Monitor `seenNonces` size metric. |

---

## 7. Verification Checklist (before marking Phase 01 ready)

- [ ] `go vet ./beacon/...` + `go test ./beacon/...` (existing suites `remote/reconnect_test.go:27-58` still assert non-blind; add `FirecrackerIsolatedRootfs` test).
- [ ] Grep `beacon/` for `ProviderLXC|ProviderKVM|DAEMON_LXC` — must be 0 hits if Option A.
- [ ] Load test `POST /api/v1/nodes/:id/files/upload?path=/tmp/test` with `bytes.Repeat('x', 120<<20)` — API returns 413 before beacon sees body; beacon never allocates >100 MiB.
- [ ] `GET /metrics` on node with 200 mock servers completes <3 s.
- [ ] `GET /servers/:id/stats` `uptimeSeconds` < `GET /api/system` daemon uptime for freshly started container.
- [ ] Disconnect panel (iptables DROP) for `2*offlineTimeout` — `ReconnectClient.State()` → `Reconnecting` not `Connected`; `RecordHeartbeatSuccess` flips only after successful `SendNodeHeartbeat`.

---

## 8. References (beacon hotspots)

- `beacon/cmd/daemon/main.go:737-765` heartbeat payload — ensure `X-Beacon-Version` (`remote/client.go:269`) still gateable after role changes.
- `beacon/internal/server/server.go:3473-3539` auth — `isStreamingUpload:3604` already expires narrowly at `files/upload`; `hostUploadLimit` streaming uses separate path.
- `beacon/internal/sftpserver/server.go:283-314` `authRequest` HMAC — matches `remote/client.go:257-276` signing; no change needed.
- `beacon/internal/transfer/protocol.go:249-320` `createSecureArchive` — `is_read_only` not relevant (tar path), but firecracker rootfs shares same `rootfs.New` isolation shape — keep pattern for overlay copy.

*End — no code modified, per task.*
