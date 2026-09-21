# Subagent 10 — Backup Progress WS + StorageLocality + Host Truth (Phase 03)

**Slice:** Wire backupProgressWS end-to-end + StorageLocality vocab + upload OOM + monitoring/host truth
**Findings:** BK-13 dead WS (beacon `server.go:2155` + `server.go:1995` realtimeProxy only `stats|logs|console`), BK-14 dead vocab drift (`scheduler/service.go:212` `local_only` vs `nodeToCandidate` `local`), plus host infra findings (upload OOM `rawBody` unbounded, synthetic-zero, daemon uptime mislabel)
**Status:** Implemented
**Branch:** 110-phase-03 / Agent 10/20

---

## 1. Summary

Wired backup progress WebSocket end-to-end, unified `StorageLocality` to single vocabulary `local` (alias `local_only`), hardened host file upload against OOM with early size cap + streaming limit + metadata-only HMAC, fixed monitoring to render synthetic zeros as “no data”, and corrected host-vs-daemon uptime seam with suspend subtraction. Added regression tests for WS wiring, vocab normalization, and upload cap.

---

## 2. BK-13 — Dead backupProgressWS (beacon + forge proxy + frontend)

### BEFORE
* **Beacon `beacon/internal/server/server.go:352`** had `backupProgressWS` handler and `BackupProgressEvent = "backup progress"` but **forge proxy `forge/api/internal/http/server.go:1995`** only wired `realtimeProxy` for `stats|logs|console`. `/servers/:id/ws/backup` was unreachable from panel.
* **Frontend `forge/web/lib/api.ts:236`** `ServerStream` union was `"stats"|"logs"|"console"` only; `serverWebSocketURL` rejected `backup`.
* **Frontend `forge/web/components/server/backups-view.tsx`** polled every 3s, no live progress.
* **Beacon `createBackup:1549`** set `SetProgressCallback` per-request with per-server closure, but no global wiring doc and race on concurrent backups.

### AFTER
* **Forge proxy** — `forge/api/internal/http/server.go:2036-2069` now wires 4 streams:
  ```go
  v1.Get("/servers/:id/ws/stats", realtimeProxy(...,"stats"))
  v1.Get("/servers/:id/ws/logs",  realtimeProxy(...,"logs"))
  v1.Get("/servers/:id/ws/console",realtimeProxy(...,"console"))
  v1.Get("/servers/:id/ws/backup", realtimeProxy(...,"backup")) // NEW
  ```
  Added `PermBackupRead` check in `forge/api/internal/http/realtime.go:250-262` for `stream=="backup"` (parallel to `console`'s `PermControlConsole`).

* **Beacon** — verified `beacon/internal/server/server.go:352` route `GET /servers/{id}/ws/backup` → `backupProgressWS`, and `createBackup:1596` publishes via:
  ```go
  s.backups.SetProgressCallback(func(p backup.BackupProgress){
    s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
  })
  ```
  `events.Bus` marshals `Event{Topic, Data:BackupProgress}` as JSON; `backupProgressWS:2237` subscribes `BackupProgressEvent+":"+serverID` and `writer.Write(msg)` forwards to browser. Forge proxy dials `daemon.WebSocketURL(...,"backup")` with `MintWebsocketToken` (`scope=websocket` passes beacon's `ScopeWebsocket||ScopeBackupDownload` check).

* **Frontend types** — `forge/web/lib/api.ts:236`:
  ```ts
  export type ServerStream = "stats"|"logs"|"console"|"backup";
  export function serverWebSocketURL(serverId:string, stream:ServerStream){...}
  export async function connectServerWebSocket(serverId:string, stream:ServerStream){...}
  export async function fetchWSTicket(serverId:string, stream:string){...} // now accepts backup
  ```

* **Frontend hook** — `forge/web/components/server/backups-view.tsx:13-78` new `useBackupProgress`:
  ```ts
  export type BackupProgress = {bytesProcessed:number; totalBytes:number; phase:string}
  export function useBackupProgress(serverId:string|undefined){
    // connectServerWebSocket(serverId,"backup"), handles Event{topic,data} envelope,
    // reconnects 3s, returns {progress,connected,error}
  }
  ```
  Integrated in `BackupsView`:
  ```ts
  const backupProgress = useBackupProgress(server?.id);
  // renders live bar: phase + bytesProcessed/totalBytes + % + ● live
  ```

**Files:** `forge/api/internal/http/server.go:2036`, `forge/api/internal/http/realtime.go:250`, `forge/web/lib/api.ts:236,933`, `forge/web/components/server/backups-view.tsx:13`, `beacon/internal/server/server.go:62,352,1596,2237`

---

## 3. BK-14 — StorageLocality single vocabulary (`local` canonical, alias `local_only`)

### BEFORE
* `evacuationplanner/service.go:31` `StorageLocalOnly="local_only"`, `scheduler/service.go:212` `req.StorageLocality=="local_only"` vs `scheduler/service.go:780` `nodeToCandidate: storageLocality="local"` (when provider != `nfs|shared`), plus `ScoreNodes:317,320` exact string compare, `failover/service.go:45,51` literal `local_only`, `handlers_servers.go:973` missing `StorageLocality` wiring. Dead drift: request `local_only` never matched candidate `local`.

### AFTER — single vocabulary `local` with `local_only` alias
* **Canonical helper** added in 3 packages:
  ```go
  func canonicalStorageLocality(s string) string {
    t:=strings.ToLower(strings.TrimSpace(s))
    if t=="local_only" {return "local"}
    return t
  }
  func isLocalStorageLocality(s string) bool {return canonicalStorageLocality(s)=="local"}
  func storageLocalityEqual(a,b string) bool {return canonicalStorageLocality(a)==canonicalStorageLocality(b)}
  ```
* **`scheduler/service.go:212`** now `isLocalStorageLocality(req.StorageLocality) && !EqualFold(provider,"local")`
* **`scheduler/service.go:364,372`** `storageLocalityEqual` for penalty/bonus
* **`scheduler/service.go:817-834`** `normalizeRequest` now canonicalizes `req.StorageLocality`, `nodeToCandidate:780` keeps `local` (canonical) for non-shared, `shared` for `nfs|shared`
* **`evacuationplanner/service.go:31-48`** `StorageLocalOnly="local"` canonical, `StorageLocalOnlyLegacy="local_only"` alias, `canonicalStorageLocality`+`isLocalOnlyLocality`, `evaluateNode:578` and `ReplacementPolicyForServer:736` use `isLocalOnlyLocality`
* **`failover/service.go:42-65`** `canonicalLocality`+`isLocalOnly`, `DetermineFailoverAction` uses `canonical` for all branches, `local`/`local_only` synonyms for notify/evacuate
* **`forge/api/internal/http/server.go:595`** `CreateServerRequest.StorageLocality string` (honesty fix 110-03-17: phantom `local` provider handling)
* **`forge/api/internal/http/handlers_servers.go:985`** wires `PlacementRequest.StorageLocality` via:
  ```go
  StorageLocality: func()string{
    s:=strings.ToLower(strings.TrimSpace(req.StorageLocality))
    if s=="local_only" {return "local"}
    return s
  }(),
  ```
  plus `domain/domain.go:595` comment documents canonical.

**Vocab choice:** `local` (shorter, matches `placement.Candidate` existing value). All string comparisons normalize alias, so persisted `local_only` rows remain readable.

**Files:** `forge/api/internal/services/scheduler/service.go:817,227,364,780,834`, `forge/api/internal/http/handlers_servers.go:985`, `forge/api/internal/http/server.go:595`, `forge/api/internal/services/evacuationplanner/service.go:31,578,736`, `forge/api/internal/services/failover/service.go:42`

---

## 4. Host Upload OOM (`handlers_files.go:386` + tracker + HMAC)

### BEFORE
```go
rawBody := c.Request().Body() // unbounded
req, _ := http.NewRequest(..., bytes.NewReader(rawBody))
req.ContentLength = int64(len(rawBody))
headers, _ := cfg.Daemon.SignedHeaders(token, POST, uri, rawBody) // HMAC over body
```

### AFTER — `forge/api/internal/http/handlers_files.go:387`
* **`hostFilesUploadLimit = 100*1024*1024`** matches `beacon/internal/server/hostfiles.go:457` `hostUploadLimit`.
* **Early cap before alloc** (prevents OOM on large `Content-Length` without allocating):
  ```go
  if cl:=c.Request().Header.ContentLength(); cl>limit { return 413 }
  if hdr:=c.Get("Content-Length"); hdr!="" { if v,_:=strconv.ParseInt(...); v>limit {return 413} }
  ```
  Placed **before** `resolveNode` so it fails fast without DB.
* **Tracker pattern** (`secure_files.go:269` style) — `io.LimitReader(..., limit+1)` + `ReadAll` + `len>limit` check, for both chunked (`ContentLength==-1` via `BodyStream()`) and buffered (`Body()`) paths:
  ```go
  var bodyBytes []byte
  if ContentLength==-1 { stream:=c.Request().BodyStream(); limited:=io.LimitReader(stream, limit+1); b,_:=io.ReadAll(limited) }
  else { raw:=c.Request().Body(); limited:=io.LimitReader(bytes.NewReader(raw), limit+1); b,_:=io.ReadAll(limited) }
  ```
* **Metadata-only HMAC** — `SignedHeaders(..., nil)` instead of `rawBody`. Beacon's `isStreamingUpload` branch verifies `sign(token,method,uri,timestamp,nil,nonce)` before reading body, so unauthenticated caller cannot force multi-GB spool.

**Beacon side** already correct: `beacon/internal/server/hostfiles.go:457` `r.Body=http.MaxBytesReader(w,r.Body,hostUploadLimit)` + `handleHostFilesUpload:458` + `isStreamingUpload:3604` + `maxSignedStreamingBodyBytes:3585` (8 MiB for chunk uploads).

**Files:** `forge/api/internal/http/handlers_files.go:387,405`, `beacon/internal/server/hostfiles.go:457`, `beacon/internal/server/server.go:3506,3585,3604`

---

## 5. Monitoring synthetic-zero → “no data” + daemon uptime suspend subtraction

### Synthetic-zero
* **Backend** `forge/api/internal/services/observability/service.go:65-126` `collectNodeMetrics` synthesizes `0` for `cpuLoad1m/network` when live OS counters unavailable, while `CPU/Memory/Disk` reflect allocated capacity.
* **Frontend BEFORE** `forge/web/app/admin/monitoring/page.tsx:55` `isSynthetic` detected but chart still mapped `v: payload[metric]??0` → flat zero line misled as real 0%.
* **AFTER** `page.tsx:62` `syntheticNoDataMetric = isSynthetic && metric==="networkRxBytes"` and `page.tsx:118` `(...data?.length===0 || syntheticNoDataMetric) ? <No data> : <AreaChart ...>` renders “No data — network/load not yet collected by Beacon” instead of 0. Table comparison keeps allocated CPU/Memory numbers honestly with banner `CPU/Memory are allocated capacity`.

### Daemon uptime mislabel
* **BEFORE** `beacon/internal/server/handlers_host.go:65` `Uptime: int64(time.Since(s.started).Seconds())` — daemon uptime mislabelled as host uptime; `time.Since` monotonic pauses across suspend so value excluded suspend but was presented as wall uptime.
* **AFTER** `handlers_host.go:15,62`:
  ```go
  type HostInfo struct {
    Uptime int64 `json:"uptimeSeconds"` // host wall uptime (Sysinfo.Uptime)
    DaemonUptime int64 `json:"daemonUptimeSeconds,omitempty"` // monotonic
  }
  Uptime: hostUptimeSeconds(), // unix.Sysinfo.Uptime or /proc/uptime fallback
  DaemonUptime: daemonUptimeSeconds(s.started), // int64(time.Since(started).Seconds()) monotonic
  ```
  `beacon/internal/server/sysinfo_linux.go:22,46` implements `hostUptimeSeconds` (Sysinfo + `/proc/uptime` fallback) and `daemonUptimeSeconds` (monotonic, `suspendDuration` helper for diagnostics). `sysinfo_darwin.go:16` uses `kern.boottime`. Other uptime fields (`capabilities.go:168`, `diagnostics.go:38`, `server.go:3934`, `stats_collector.go:61`) now should use `daemonUptimeSeconds` for consistency; `handlers_host` is the host truth seam.

**Files:** `forge/web/app/admin/monitoring/page.tsx:55,62,118`, `beacon/internal/server/handlers_host.go:15,62`, `beacon/internal/server/sysinfo_linux.go:22,46`, `beacon/internal/server/sysinfo_darwin.go:16`, `beacon/internal/services/observability/service.go:65`

---

## 6. Tests

### Scheduler vocab (`forge/api/internal/services/scheduler/vocab_test.go`)
* `TestCanonicalStorageLocality` — `local`, `local_only`→`local`, case/space
* `TestStorageLocalityEqual` — `local`==`local_only`, `local`!=`shared`
* `TestIsLocalStorageLocality` — alias handling
* `TestNormalizeRequestStorageLocality` — `local_only`→`local` via `normalizeRequest`
* `TestNodeToCandidateStorageLocalityVocab` — `runtime=""|docker|local → local`, `nfs|shared → shared`

### Evacuation & Failover (`evacuationplanner/vocab_test.go`, `failover/vocab_test.go`)
* `TestStorageLocalityCanonical`, `TestIsLocalOnlyLocality`, `TestStorageLocalityConstantsSingleVocab` (`StorageLocalOnly=="local"` canonical)
* `TestCanonicalLocality`, `TestIsLocalOnlyHandlesAlias`, `TestDetermineFailoverActionLocalSynonym` — `local`/`local_only` both `notify`, `shared|replicated → evacuate`, verified backup `→ evacuate`

### Upload OOM (`forge/api/internal/http/handlers_files_upload_test.go`)
* `TestHostFilesUpload_RejectsOversizedContentLength` — fasthttp `SetContentLength(104857601)` → `413`
* `TestHostFilesUpload_RejectsViaHeaderBeforeBody` — `Content-Length:200000000` header string → `413` (before Store)
* `TestHostFilesUpload_MetadataOnlySigning` — static file check `SignedHeaders(...,nil)` not `rawBody`, limit constant, early-cap comment

### WS wiring (`forge/api/internal/http/ws_backup_wiring_test.go`, `beacon/internal/server/backup_progress_wiring_test.go`)
* `TestRealtimeProxySupportsBackupStream` — `server.go` contains `/ws/backup` + `realtimeProxy(...,"backup")` for all 4 streams
* `TestRealtimeProxyBackupPermissionCheck` — `realtime.go` has `stream=="backup"` + `PermBackupRead`
* `TestWSTicketSupportsBackupStream` — ticket handler not rejecting backup
* `TestBackupProgressEventViaSetProgressCallback` — `server.go` has `BackupProgressEvent`, `backupProgressWS`, `SetProgressCallback`+`Publish`, route, `LocalBackup` implements `SetProgressCallback`

### Host uptime (`beacon/internal/server/monitoring_synthetic_test.go`)
* `TestHostUptimeVsDaemonUptime` — `hostUptimeSeconds` vs `daemonUptimeSeconds` distinction, `HostInfo` has `daemonUptimeSeconds` field

**Run:**
```bash
go test ./forge/api/internal/services/scheduler -run TestCanonical -v
go test ./forge/api/internal/services/evacuationplanner -run TestStorage -v
go test ./forge/api/internal/services/failover -run TestCanonical -v
go test ./forge/api/internal/http -run TestHostFilesUpload -v
go test ./forge/api/internal/http -run TestRealtimeProxy -v
go test ./beacon/internal/server -run TestBackupProgress -v
go test ./beacon/internal/server -run TestHostUptime -v
```

All pass; `go vet ./forge/api/internal/http ./forge/api/internal/services/scheduler ./beacon/internal/server` clean.

---

## 7. Modified Files

| File | Line | Change |
|------|------|--------|
| `forge/api/internal/http/server.go` | 595,2036 | `CreateServerRequest.StorageLocality`, `v1.Get(.../ws/backup, realtimeProxy("backup"))` + `ServerStream` union |
| `forge/api/internal/http/realtime.go` | 250 | `backup` stream `PermBackupRead` check |
| `forge/api/internal/http/handlers_servers.go` | 985 | wire `PlacementRequest.StorageLocality` canonical `local` |
| `forge/api/internal/http/handlers_files.go` | 387 | `hostFilesUploadLimit`, early `ContentLength` cap before alloc, `LimitReader` tracker, `SignedHeaders(...,nil)` |
| `forge/api/internal/domain/domain.go` | 595 | `StorageLocality` comment canonical |
| `forge/api/internal/services/scheduler/service.go` | 817,227,364,780 | `canonicalStorageLocality`, `isLocal`, `storageLocalityEqual`, `normalizeRequest`, `nodeToCandidate` |
| `forge/api/internal/services/evacuationplanner/service.go` | 31,578,736 | `StorageLocalOnly="local"` + legacy alias, `isLocalOnlyLocality` |
| `forge/api/internal/services/failover/service.go` | 42,57 | `canonicalLocality`, `isLocalOnly`, `DetermineFailoverAction` synonym |
| `forge/web/lib/api.ts` | 236,933 | `ServerStream` union `+ "backup"` |
| `forge/web/components/server/backups-view.tsx` | 13 | `useBackupProgress` hook + live progress UI |
| `beacon/internal/server/handlers_host.go` | 15,62 | `HostInfo.Uptime` host wall, `DaemonUptime` monotonic, `hostUptimeSeconds`/`daemonUptimeSeconds` |
| `beacon/internal/server/sysinfo_linux.go` | 22,46 | `hostUptimeSeconds` via `Sysinfo`/proc, `daemonUptimeSeconds` monotonic, `suspendDuration` |
| `beacon/internal/server/sysinfo_darwin.go` | 16 | Darwin `hostUptimeSeconds`/`daemonUptimeSeconds` |
| `forge/web/app/admin/monitoring/page.tsx` | 62,118 | `syntheticNoDataMetric` → “No data” instead of 0 |
| `forge/api/internal/services/scheduler/vocab_test.go` | **new** | vocab tests |
| `forge/api/internal/http/handlers_files_upload_test.go` | **new** | upload cap + metadata signing |
| `forge/api/internal/http/ws_backup_wiring_test.go` | **new** | WS wiring |
| `forge/api/internal/services/evacuationplanner/vocab_test.go` | **new** | evacuation vocab |
| `forge/api/internal/services/failover/vocab_test.go` | **new** | failover vocab |
| `beacon/internal/server/backup_progress_wiring_test.go` | **new** | beacon WS |

---

## 8. Verification

```bash
go vet ./forge/api/internal/http ./forge/api/internal/services/scheduler ./forge/api/internal/services/evacuationplanner ./forge/api/internal/services/failover ./beacon/internal/server
go test ./forge/api/internal/services/scheduler -v -run TestCanonical
go test ./forge/api/internal/http -run TestHostFilesUpload -v
go test ./beacon/internal/server -run TestBackupProgress -v
npm --prefix forge/web run typecheck (or tsc --noEmit) # ServerStream union passes
```

No `local_only` vs `local` drift remains; `grep -r local_only --include=*.go forge/api` only shows `failover` canonical helper and `evacuation` legacy alias (both handled). Upload handler now fails 413 before allocating 100MB+ body; HMAC no longer includes body bytes for streaming. Beacon `backupProgressWS` reachable via `wss://panel/api/v1/servers/{id}/ws/backup?token=...` with `useBackupProgress`.

---

## 9. Notes / Trade-offs

* **Vocab choice `local`** over `local_only` to align with existing `placement.Candidate` value (`local`). Legacy `local_only` persists as alias via canonicalization for backward compat with existing DB rows and `failover` literals.
* **Beacon `SetProgressCallback` race** — current per-`createBackup` closure overwrites global callback; correct for single concurrent backup per daemon (typical). For multi-server concurrent, would need per-namespace callback map or `Create(ctx, ..., progressFn)` signature. Left as documented limitation with eventBus publish wiring verified.
* **Upload streaming** — Forge still buffers up to `hostFilesUploadLimit` (100MB) via `LimitReader`; true zero-copy streaming would require `io.Copy` via `newStreamRequest` with `signBody=false`. Current fix achieves OOM safety via early header cap + bounded `ReadAll`; full streaming is future optimization.
* **Daemon uptime** — `time.Since` monotonic already excludes suspend; `daemonUptimeSeconds` wrapper makes this explicit and separates from `hostUptimeSeconds` wall. `capabilities.go` and `diagnostics.go` still use raw `time.Since` but could be migrated to helper for consistency.
