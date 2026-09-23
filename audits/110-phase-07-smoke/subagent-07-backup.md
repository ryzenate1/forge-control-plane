# Subagent 07 — Backup/Storage Smoke Test (Phase 07)

**Date:** 2026-08-24
**Focus:** Backup/Storage — retention OR, encryption streaming, flock vs map, pg_advisory locks, frontend live progress, WS wiring
**Agent:** 07/10 (parallel smoke)

---

## 1. Test Execution

### 1.1 `forge/api/internal/services/backup -run TestRetention`
```bash
go test ./forge/api/internal/services/backup -run TestRetention -count=1 2>&1 | tail -n 20
```
- **Result (normalized `go.work` module = `gamepanel/forge/internal/...`):**
  ```
  ok  	gamepanel/forge/internal/services/backup	0.709s [no tests to run]
  ```
- Root cause: exact regex `TestRetention` matches none — suite uses `TestService_RetentionEnforcement_DeletesOldBackups` etc. Corrected run:
  ```bash
  go test ./forge/api/internal/services/backup -run "TestService_Retention" -count=1 -v
  ```
  ```
  === RUN   TestService_RetentionEnforcement_DeletesOldBackups
  --- PASS: TestService_RetentionEnforcement_DeletesOldBackups (0.00s)
  PASS
  ok  	gamepanel/forge/internal/services/backup	0.736s
  ```
- Wider run:
  ```bash
  go test ./forge/api/internal/services/backup -run "TestService_Retention|TestScenario_Retention" -count=1 -v
  # also covers TestScenario_RetentionEnforcement, TestScenario_PolicyRetentionDays
  # all PASS (full suite 7.746s PASS)
  ```

### 1.2 `beacon/internal/backup -run TestFlock`
```bash
go test ./beacon/internal/backup -run TestFlock -count=1 -v 2>&1 | tail -n 20
```
```
=== RUN   TestFlockPreventsConcurrentBackup
--- PASS: TestFlockPreventsConcurrentBackup (0.00s)
PASS
ok  	gamepanel/beacon/internal/backup	0.288s
```
- **PASS**

### 1.3 `beacon/internal/backup -run TestEncrypt`
```bash
go test ./beacon/internal/backup -run TestEncrypt -count=1 2>&1 | tail -n 20
# beacon has no TestEncrypt* — only retention/flock/local/s3 tests
ok  	gamepanel/beacon/internal/backup	0.289s [no tests to run]
```
Corrected forge-side encrypt tests (actual encryption lives in `forge/api/internal/services/backup`):
```bash
go test ./forge/api/internal/services/backup -run "Encrypt" -count=1 -v
```
```
=== RUN   TestEncryptDecryptRoundTrip
--- PASS: TestEncryptDecryptRoundTrip (0.00s)
=== RUN   TestEncryptedDataNotPlaintext
--- PASS: TestEncryptedDataNotPlaintext (0.00s)
=== RUN   TestEncryptThenCompressRoundTrip
--- PASS: TestEncryptThenCompressRoundTrip (0.00s)
=== RUN   TestEncryptLargeData
--- PASS: TestEncryptLargeData (0.00s)
=== RUN   TestEncryptReaderStreamingChunked
--- PASS: TestEncryptReaderStreamingChunked (0.00s)
=== RUN   TestEncryptReaderStreamingNoOOM
--- PASS: TestEncryptReaderStreamingNoOOM (0.00s)
PASS
ok  	gamepanel/forge/internal/services/backup	0.513s
```
- **PASS** — streaming chunked + NoOOM regression both pass.

### 1.4 Full `beacon/internal/backup` suite
```bash
go test ./beacon/internal/backup -run Test -count=1 -v 2>&1 | tail -n 100
```
```
=== RUN   TestLocalBackupLifecycleAndRestore --- PASS (0.07s)
=== RUN   TestLocalRestoreWithoutTruncation --- PASS (0.02s)
=== RUN   TestLocalRestoreWithPathsRestoresOnlyListedPaths --- PASS (0.02s)
=== RUN   TestLocalRestoreRejectsMaliciousArchivesAndPreservesLiveData (7 subtests) --- PASS
=== RUN   TestLocalRestoreChecksumMismatchPreservesLiveData --- PASS
=== RUN   TestLocalBackupMigratesLegacyStorageSafely --- PASS
=== RUN   TestRecoverRestoreJournalsRestoresOriginalLiveData --- PASS
=== RUN   TestLocalCreateRejectsBackupRootInsideServerRoot --- PASS
=== RUN   TestFlockPreventsConcurrentBackup --- PASS
=== RUN   TestGCPartialReapsOrphan --- PASS
=== RUN   TestS3CreateRetrySuccessAfterFailureAndCleansStaging --- PASS
=== RUN   TestS3ListPaginatesAndPreservesChecksumMetadata --- PASS
=== RUN   TestS3DownloadRejectsChecksumMismatchAndCleansStaging --- PASS
=== RUN   TestS3DownloadAbortsOnInsufficientDiskSpace --- PASS
=== RUN   TestConfigureS3OptionsHonorsPathStyleWithAndWithoutEndpoint (3 subtests) --- PASS
=== RUN   TestRetentionPolicy --- PASS
=== RUN   TestRetentionPolicyRejectsNegativeCounts --- PASS
=== RUN   TestRetentionPolicySafetyRailKeepsMostRecentBackup --- PASS
=== RUN   TestRetentionPolicyOrSemantics --- PASS
=== RUN   TestRetentionPolicyNoActiveRulesKeepsAll --- PASS
=== RUN   TestScheduler_* (5 tests) --- PASS
=== RUN   TestSQLiteStore --- PASS
=== RUN   TestVerifyBackup --- PASS
PASS ok gamepanel/beacon/internal/backup 0.433s
```
- 26 tests PASS, 0 FAIL.

### 1.5 Full `forge/api/internal/services/backup` suite
```bash
go test ./forge/api/internal/services/backup -count=1 -v 2>&1 | tail -n 60
```
```
PASS ok gamepanel/forge/internal/services/backup 7.746s
```
Parallel `forge/...` fails expectedly due to `go.work` using `./beacon` + `./forge/api` (pattern `./forge/...` invalid) — not a regression.

### 1.6 WS wiring static tests
```bash
go test ./beacon/internal/server -run TestBackupProgress -count=1 -v
go test ./forge/api/internal/http -run TestRealtimeProxy -count=1 -v
```
```
=== RUN   TestBackupProgressEventViaSetProgressCallback --- PASS
ok  	gamepanel/beacon/internal/server	0.522s

=== RUN   TestRealtimeProxySupportsBackupStream --- PASS
=== RUN   TestRealtimeProxyBackupPermissionCheck --- PASS
ok  	gamepanel/forge/internal/http	1.420s
```

---

## 2. Code Fix Verification (4 checks)

### 2.1 `encryption.go` streaming fix (no `io.ReadAll` for large)

**File:** `forge/api/internal/services/backup/encryption.go:329-354`

```go
// EncryptReaderWithAAD returns a streaming encryption reader with server_id:backup_name AAD binding.
func EncryptReaderWithAAD(r io.Reader, key []byte, serverID, backupName string) (io.Reader, error) {
    // ...
    pr, pw := io.Pipe()
    go func() {
        defer pw.Close()
        var err error
        if isEncryptionV2Enabled() {
            err = streamEncryptV2(pw, r, key, aad) // 1 MiB chunked
        } else {
            err = streamEncryptLegacy(pw, r, key)
        }
        if err != nil { pw.CloseWithError(err) }
    }()
    return pr, nil
}
```

- `streamEncryptV2` / `streamEncryptLegacy` at `encryption.go:459`, `549`: **chunked 1 MiB loop** with `buf := make([]byte, chunkSize)` (`chunkSize = 1 << 20` at `encryption.go:27`), `io.ReadFull` per chunk, writing `nonce + uint32(len(ct)) + ct` per chunk. No `io.ReadAll` in hot path.
- `streamDecryptV2` at `encryption.go:622` likewise reads nonce/ctLen/ct per chunk, `gcm.Open` streaming.
- **Residual `io.ReadAll`:** 
  - `encryption.go:425` inside `DecryptReaderWithAAD` — **only** legacy single-shot fallback (old backups before chunked framing). Comment at `encryption.go:373` explicitly acknowledges OOM risk confined to old backups; V2 and legacy-chunked paths remain streaming.
  - `compression.go:200,239,252` and `storage_s3.go:130,169,458,623` use `io.ReadAll` for compression helpers and S3 download fallback — out of scope for backup encryption streaming; S3 adapter also supports `UploadStream` (see `service.go:381-391`).

**Verdict:** **PASS** — Kopia-pattern 1 MiB chunked streaming via `io.Pipe` eliminates unbounded buffering for large backups. Legacy `io.ReadAll` is isolated to backward-compat fallback.

### 2.2 `retention.go` union OR

**Beacon:** `beacon/internal/backup/retention.go:60-118`

```go
// Union: a backup is kept if ANY active rule keeps it (OR semantics).
hasActive := p.MaxAge > 0 || p.MaxBackups > 0 || p.KeepDaily > 0 || p.KeepWeekly > 0 || p.KeepMonthly > 0

// Rule 1: MaxAge — keep younger
// Rule 2: KeepDaily/Weekly/Monthly — at most N per period (break after)
// Rule 3: MaxBackups — keep newest N (union, not intersect)
// Safety rail: keep[backups[0].ID] = true always
```

- **OR semantics validated** by `TestRetentionPolicyOrSemantics` (PASS). No intersection (AND) bug.
- Safety rail + `!hasActive => keep all` prevents wipeout.

**Forge convergence (2 locations):**

- `forge/api/internal/services/backup/artifact.go:734-741`:
  ```go
  // Converged RetentionEngine: keep if ANY rule keeps (OR), delete only if exceeds ALL active rules
  withinCount := policy.MaxBackups <= 0 || index < policy.MaxBackups
  withinAge := cutoff.IsZero() || !artifact.CreatedAt.Before(cutoff)
  if artifact.IsLocked || withinCount || withinAge { continue } // OR
  ```

- `forge/api/internal/services/backup/service.go:851-858`:
  ```go
  // Union OR: keep if any retention rule keeps (converged RetentionEngine)
  withinCount := policy.MaxBackups <= 0 || index < policy.MaxBackups
  withinAge := policy.RetentionDays <= 0 || now.Sub(backup.CreatedAt) <= ...
  keep[backup.Name] = backup.IsLocked || withinCount || withinAge // OR
  ```

**Verdict:** **PASS** — all three engines (beacon + forge artifact + forge service) converge on OR (union) semantics with comments citing `RetentionEngine`.

### 2.3 `store_backups.go` `pg_advisory_xact_lock`

**File:** `forge/api/internal/store/store_backups.go:240-335`

```go
func (s *Store) CleanupOldBackups(ctx context.Context, retentionDays int, autoCleanup bool) (int, error) {
    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)
    if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:global', 0))`); err != nil { return 0, err }
    // ... DELETE .partial GC + DELETE old ...
    return int(commandTag.RowsAffected()), tx.Commit(ctx)
}

func (s *Store) CleanupOldBackupsForServer(ctx context.Context, serverID string, retentionDays int, backupLimit int) (int, error) {
    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)
    if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:'||$1, 0))`, serverID); err != nil { return 0, err }
    // GC .partial + union DELETE (retentionDays OR over-limit via UUID IN subquery)
    // comment: "RetentionEngine OR: keep if any rule keeps, delete only if exceeds ALL active thresholds"
}
```

- Uses `pg_advisory_xact_lock(hashtextextended(...))` — transaction-scoped, auto-released on commit/rollback, safe for replica serialization.
- Global lock key `backup_prune:global`; per-server key `backup_prune:<serverID>` prevents cross-server contention while serializing prune mark-sweep + S3 sweep + `.partial` reaper per server.

**Verdict:** **PASS** — correctly serializes prune across replicas.

### 2.4 `local.go` `flock` vs `map`

**File:** `beacon/internal/backup/local.go:90-164` + `beacon/internal/backup/flock_unix.go:10`, `flock_windows.go:12`

```go
func (l *LocalBackup) lockNamespace(namespace string) func() {
    // Cross-process advisory lock via flock on backupRoot/<ns>/.backup.lock
    // Replaces in-process map to fix BK-09 (lockNamespace in-process only).
    dir, err := l.namespaceDir(namespace, true)
    if err != nil { /* fallback to map */ }
    lockPath := filepath.Join(dir, ".backup.lock")
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
    if err != nil { /* fallback to map */ }
    if err := flockLock(f); err != nil { _ = f.Close(); /* fallback */ }
    return func() { _ = flockUnlock(f); _ = f.Close() }
}
```

- Unix: `syscall.Flock(LOCK_EX)` / `LOCK_UN` / `LOCK_NB` at `flock_unix.go:10-19`.
- Windows: `sync.Mutex` fallback at `flock_windows.go:12` (noted as limitation).
- **Fallback tiers:** (1) flock file → (2) in-process `namespaceOps` map + `sync.Mutex` if dir/file creation fails or flock fails — prevents total outage while preserving best-effort cross-process isolation.
- Also used for `GCPartial` at `local.go:190` per-namespace reap.

**Verdict:** **PASS** — cross-process `flock` primary with map fallback correctly fixes BK-09 (previous in-process-only lock). Windows caveat documented.

---

## 3. Frontend Checks

### 3.1 `forge/web/app/admin/backups` page

**File:** `forge/web/app/admin/backups/page.tsx:1-959`

- Route: `app/admin/backups/page.tsx` — **not** `app/admin/backups` bare directory mismatch in task description; correct path is `forge/web/app/admin/backups/page.tsx`.
- 5 tabs: `overview | policies | jobs | artifacts | restores` (line 892). Uses `useQuery` polling `refetchInterval: 30_000` for configs/jobs/artifacts/restores/storage-providers (lines 256-290).
- **No live WS** — admin page is polling-only. `progress` rendered via `renderProgress` (line 421) using static `job.progress` / `restore.progress` fields, not WS. No `useBackupProgress` / `connectServerWebSocket` import.
- CRUD: create config/job, cancel/delete job, lock/unlock artifact, download artifact, restore, delete restore — all present.
- Depth of coverage matches `admin/cloud` counterpart (system status card, Quick Actions).

**Verdict:** **PASS (with note)** — admin page exists and functional, but **live progress is polling-based, not WS**. Task expectation of `backups-view.tsx live progress` applies to *server* view, not admin (see next).

### 3.2 `forge/web/components/server/backups-view.tsx` live progress

**File:** `forge/web/components/server/backups-view.tsx:20-91`, `155`, `174-183`

```tsx
export function useBackupProgress(serverId: string | undefined) {
  const [progress, setProgress] = useState<BackupProgress | null>(null);
  const [connected, setConnected] = useState(false);
  // ...
  ws = await connectServerWebSocket(serverId, "backup");
  ws.onmessage = (event) => {
    const payload = raw?.data ?? raw; // Beacon eventBus { topic, data: BackupProgress }
    if (payload && typeof payload.bytesProcessed === "number") {
      setProgress({ bytesProcessed, totalBytes, phase });
    }
  };
  ws.onclose = () => { reconnectTimer = setTimeout(connect, 3000); };
}

export function BackupsView({ server }: { server?: ApiServer }) {
  const backupProgress = useBackupProgress(server?.id); // line 155
  // line 174-183: renders phase + bytesProcessed/totalBytes % + connected dot
}
```

- Consumes `connectServerWebSocket(serverId, "backup")` from `@/lib/api`.
- Beacon payload shape `BackupProgress{BytesProcessed, TotalBytes, Phase}` matched.
- Auto-reconnect 3 s on close/error, cancels on unmount.
- UI: `● live backup progress` dot (emerald when connected), phase + formatted bytes + percent.

**Verdict:** **PASS** — full live WS wiring present on server detail page; admin page intentionally polling.

---

## 4. WebSocket Wiring

### 4.1 Beacon `backupProgressWS`

**File:** `beacon/internal/server/server.go:95`, `407`, `2379-2432`

```go
const BackupProgressEvent = "backup progress" // 95
mux.HandleFunc("GET /servers/{id}/ws/backup", server.backupProgressWS) // 407 (was 2155 pre-refactor)

func (s *Server) backupProgressWS(w http.ResponseWriter, r *http.Request) { // 2379
    // auth via authenticateWebSocket, scope check ScopeWebsocket|ScopeBackupDownload (2392)
    conn, _ := websocketUpgrader.Upgrade(w, r, nil)
    defer conn.Close(); defer s.trackWebSocket(r, conn)()
    configureWebSocket(conn)
    serverID := r.PathValue("id")
    if claims.ServerID != serverID { ... }
    ch := s.eventBus.Subscribe(BackupProgressEvent + ":" + serverID) // 2416
    defer s.eventBus.Unsubscribe(BackupProgressEvent+":"+serverID, ch)
    for { select { case msg := <-ch: writer.Write(msg) } }
}
```

- **Publish side:** `server.go:1762-1765` inside `createBackup`:
  ```go
  s.backups.SetProgressCallback(func(p backup.BackupProgress) {
      s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
  })
  ```
  `LocalBackup.reportProgress` at `local.go:244` invokes `progress` func with `BackupProgress` per chunk.
- **Test:** `beacon/internal/server/backup_progress_wiring_test.go:26` asserts `BackupProgressEvent`, `backupProgressWS`, `SetProgressCallback` + `eventBus.Publish`, route — PASS (see Section 1.6).

**Verdict:** **PASS** — per-server topic `backup progress:<serverID>` correctly bridges adapter progress → eventBus → WS.

### 4.2 Forge `realtimeProxy` includes `backup`

**Files:** `forge/api/internal/http/realtime.go:124-273`, `forge/api/internal/http/server.go:2282-2295`

- **Registration** at `server.go:2282`:
  ```go
  v1.Get("/servers/:id/ws/backup", requireRealtimeServices(cfg), wsOriginMiddleware(cfg),
      fiberws.New(realtimeProxy(cfg, wsTickets, "backup"), fiberws.Config{ ... }))
  ```
  Alongside `stats` (2249), `logs` (2260), `console` (2271) — all four streams present.

- **Proxy permission gate** at `realtime.go:263-273`:
  ```go
  if stream == "backup" {
      backupAllowed, _ := cfg.Store.UserCanAccessServer(ctx, client.Params("id"), userID, userRole, store.PermBackupRead)
      if !backupAllowed { _ = client.WriteJSON(... "missing server permission: " + store.PermBackupRead); return }
  }
  ```
  Mirrors console's `PermControlConsole` gate; prevents `websocket.connect`-only snoop.

- **Upstream dial:** `cfg.Daemon.WebSocketURL(target.NodeURL, target.ServerID, "backup")` + `MintWebsocketToken` (lines 298-314) — same as other streams, beacon validates via `ScopeWebsocket`/`ScopeBackupDownload`.

- **Tests:** `forge/api/internal/http/ws_backup_wiring_test.go:10-20` PASS for `realtimeProxy` backup route + `PermBackupRead`.

**Verdict:** **PASS** — `realtimeProxy` fully supports `backup` stream with auth, Origin, ticket, and per-permission checks; panel→beacon→browser pipe complete.

---

## 5. Summary Table

| Check | Spec | Evidence | Result |
|-------|------|----------|--------|
| `go test .../backup -run TestRetention` | smoke passes | Forge `TestService_RetentionEnforcement_DeletesOldBackups` PASS; Beacon 5 retention tests PASS | **PASS** |
| `go test .../backup -run TestFlock` | flock prevents concurrent | `TestFlockPreventsConcurrentBackup` PASS (0.00s) | **PASS** |
| `go test .../backup -run TestEncrypt` | encryption round-trip | Forge 6 encrypt tests PASS including `StreamingChunked` + `NoOOM` | **PASS** |
| `encryption.go streaming fix` | no `io.ReadAll` for large | `EncryptReaderWithAAD` via `io.Pipe` + 1 MiB `streamEncryptV2`; V2 decrypt chunked; legacy `ReadAll` isolated to old-backup fallback (`:425`) | **PASS** |
| `retention.go union OR` | keep if ANY rule | Beacon `retention.go:60` + Forge `artifact.go:734` + `service.go:851` all OR; `TestRetentionPolicyOrSemantics` PASS | **PASS** |
| `store_backups.go pg_advisory lock` | serialize prune | `pg_advisory_xact_lock(hashtextextended('backup_prune:…'))` global + per-server, tx-scoped (2282-335 lines) | **PASS** |
| `local.go flock vs map` | cross-process lock | `lockNamespace` primary `flock` on `.backup.lock` + map fallback; BK-09 fix comment | **PASS** |
| `forge/web/app/admin/backups` | admin backups page | `app/admin/backups/page.tsx` 959 lines, 5 tabs, polling 30s | **PASS (polling)** |
| `backups-view.tsx live progress` | WS live progress | `useBackupProgress` via `connectServerWebSocket(serverId,"backup")`, phase+bytes+% + reconnect | **PASS** |
| `beacon/server.go backupProgressWS` | WS handler exists | `server.go:2379` (migrated from :2155), `BackupProgressEvent` + `SetProgressCallback→Publish` + `GET /servers/{id}/ws/backup` | **PASS** |
| `realtimeProxy includes backup` | panel proxy | `server.go:2282` `realtimeProxy(...,"backup")` + `realtime.go:263` `PermBackupRead` gate | **PASS** |

**Overall:** **11/11 PASS** (1 with polling-not-WS note). No smoke failures detected. All Phase-03 fixes (Kopia chunked streaming, OR retention, advisory locks, flock, BK-13 WS) remain intact.

---

## 6. Gaps / Notes (non-blocking)

1. **Task path drift:** task lists `forge/web/app/admin/backups, backups-view.tsx` — actual admin page is `app/admin/backups/page.tsx` and live WS lives in `components/server/backups-view.tsx` (server detail), not admin. Not a bug.
2. **Residual `io.ReadAll` in non-hot paths:** `encryption.go:425` (legacy single-shot decrypt fallback), `compression.go`, `storage_s3.go` — acceptable for old backups / S3 helpers; `service.go:387` fallback only when adapter lacks `UploadStream`.
3. **Beacon `TestEncrypt*` naming:** beacon module has no `TestEncrypt*` — tests live in forge; task command expectedly returns `[no tests to run]` for beacon encrypt.
4. **Windows `flock` limitation:** `flock_windows.go` falls back to in-process mutex (no cross-process file lock on Windows) — acceptable for Linux daemons.

---

## 7. Commands Reproduced (copy-paste)

```bash
# Exact task commands
go test ./forge/api/internal/services/backup -run TestRetention -count=1 2>&1 | tail -n 20
go test ./beacon/internal/backup -run TestFlock -count=1 2>&1 | tail -n 20
go test ./beacon/internal/backup -run TestEncrypt -count=1 2>&1 | tail -n 20

# Corrected (actual suite names)
go test ./forge/api/internal/services/backup -run "TestService_Retention" -count=1 -v 2>&1 | tail -n 20
go test ./beacon/internal/backup -run TestRetention -count=1 -v 2>&1 | tail -n 20
go test ./forge/api/internal/services/backup -run Encrypt -count=1 -v 2>&1 | tail -n 20
go test ./beacon/internal/backup -run TestFlock -count=1 -v 2>&1 | tail -n 20
go test ./beacon/internal/server -run TestBackupProgress -count=1 -v 2>&1 | tail -n 20
go test ./forge/api/internal/http -run TestRealtimeProxy -count=1 -v 2>&1 | tail -n 20
```
