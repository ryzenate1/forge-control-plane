# Phase 03 — Subagent 05: Backup Operations UX / Progress / Disaster Recovery

**Dimension:** BACKUP OPERATIONS / UX / PROGRESS / DISASTER RECOVERY  
**Cluster:** `kopia`, `restic` (2-project backup reference)  
**Forge scope:** `forge/web/app/admin/backups`, `forge/web/app/console/servers/[id]/backups`, `forge/web/components/server/backups-view`, `forge/api/internal/http/handlers_backups` + `handlers_servers` backup routes, `beacon/internal/backup` + `beacon/internal/server/backupProgressWS`, docs/onboarding/recovery coordinator interplay  
**Date:** 2026-08-23  
**Author:** subagent 05

---

## 1. Executive Summary

Forge has **three disjoint backup stacks that never converge in the UI**, so the backup experience reads as dead even though the plumbing exists:

1. **Per-server tar/zip path** (`beacon/internal/backup` → `handlers_servers.go` POST `/servers/:id/backups`) — fire-and-forget `pending` record + async daemon `CreateBackup` with a 15-minute goroutine. Poll-only feedback.
2. **Policy/job/artifact/restore stack** (`forge/api/internal/services/backup` + `handlers_backup_extended.go` `/admin/backups/*` + `/servers/:id/backups/policies`) — rich `progressPercentage/currentPhase/bytesProcessed` state, verified artifacts, retention, GCS/Azure adapters — but only visible on two admin-adjacent pages (`/admin/backups`, `/console/backups`).
3. **Beacon-local WebSocket stream** (`beacon/internal/server/server.go:351` `GET /servers/{id}/ws/backup` + `BackupProgressEvent`) — a correctly implemented push channel that **no Forge API proxy route and no `forge/web` component ever consumes**.

Reference baselines (Kopia `cli_progress.go`/`upload_progress.go`, Restic `internal/ui/backup/{json,progress}.go`) both provide **throttled, incremental, multi-signal progress with estimation, ETA, per-file error accounting, and machine-parseable JSON streaming**. Forge degrades all of that to a single `status` string and a coarse `progress: number` rendered as `"{12}%"`, with `TotalBytes` permanently `0` on the data plane.

The disaster-recovery coordinator (`forge/api/internal/services/recovery/backup_restore.go:32`) implements a **double-verification gate** (ListBackups + full download+SHA256 + `RestoreBackup`) with journal/rollback semantics, but it is not surfaced from any user-facing backup list. The console drill button (`/console/backups` → `POST /admin/backups/restore`) is the only bridge, and it is undiscoverable from the daily `BackupsView` where operators actually live. Docs/onboarding describe backups as "configure after install" and never enforce a shared-storage preflight, so multi-node restore assumptions fail silently.

**Verdict:** Capabilities are not missing — they are **invisible, unbridged, and unestimated**, which is worse than not existing. Users correctly conclude the system is inert.

---

## 2. Scope & Methodology

### Files inspected

**Reference**

- `reference/backup/kopia/cli/cli_progress.go:49-202` — spinner, 300 ms throttle, `hashedFiles/cachedFiles/uploadedBytes` counters, `timetrack.Estimator` for `%` + `Remaining`.
- `reference/backup/kopia/snapshot/upload/upload_progress.go:79-202` — `Progress` interface (`HashingFile`, `FinishedHashingFile`, `HashedBytes`, `UploadedBytes`, `CachedFile`, `Error(isIgnored)`, `EstimatedDataSize`), `Counters{TotalCachedBytes,TotalHashedBytes,TotalUploadedBytes,EstimatedBytes,FatalErrorCount,IgnoredErrorCount,CurrentDirectory,LastErrorPath}` + `CountingUploadProgress.UITaskCounters`.
- `reference/backup/kopia/cli/command_snapshot_create.go:55,409` — `jsonOutput` flag + `snapshot create --json` path (`jsonIndentedBytes`).
- `reference/backup/kopia/cli/command_snapshot_verify.go:56-99` — `verify --json` with `ShowFinalStats` suppression, `VerifierOptions{VerifyFilesPercent, Parallelism, MaxErrors, JSONStats}`.
- `reference/backup/restic/internal/ui/backup/json.go:42-64,199-268` — JSON streaming: `message_type: status|error|verbose_status|summary`, `statusUpdate{percent_done,total_bytes,bytes_done,seconds_remaining,current_files}`, `errorUpdate{during: scan|archival}`, `summaryOutput{data_added,data_added_packed,data_blobs,tree_blobs,total_duration,snapshot_id}`.
- `reference/backup/restic/internal/ui/backup/progress.go:52-84,88-165` — `ProgressPrinter` interface, `rateEstimator`, `NewProgress` throttled via `progress.CalculateProgressInterval`, `ReportTotal`/`StartFile`/`CompleteItem` with scan-finished ETA (`todo/rate`) and 1024 B/s cutoff.

**Forge — user surfaces**

- `forge/web/components/server/backups-view.tsx:13-222` — **daily-use backup tab**.
- `forge/web/app/console/servers/[id]/backups/page.tsx:1-9` + `forge/web/app/server/[id]/backups/page.tsx:1-14` — wrappers.
- `forge/web/app/console/backups/page.tsx:98-1047` — **policy/storage/artifact/drill console**.
- `forge/web/app/admin/backups/page.tsx:1-923` — **admin 5-tab backup & recovery**.
- `web/components/backup-manager.tsx:56-125` — legacy docs-site backup card (separate `web/` app, not panel).

**Forge — APIs & beacon**

- `forge/api/internal/http/handlers_servers.go:1912-2240` — server-scoped backup CRUD, `GET /backups/verify`, `GET /backups/storage/download`.
- `forge/api/internal/http/handlers_backup_extended.go:1-556` — policy + `/admin/backups/{configs,jobs,artifacts,restores,storage-providers,status}`.
- `forge/api/internal/services/backup/job.go:17-422` — `BackupJob{ProgressPercentage, CurrentPhase, BytesProcessed, TotalBytes, ErrorMessage, RetryCount}`.
- `forge/api/internal/services/backup/restore.go:17-514` — `BackupRestore{ProgressPercentage, CurrentPhase, VerificationStatus, CanRollback, RollbackArtifactID}`.
- `forge/api/internal/services/backup/artifact.go:22-516` — `BackupArtifact{FileHash, IsVerified, IsLocked, Manifest}` + `Verify`.
- `forge/api/internal/services/recovery/backup_restore.go:32-114` — `DaemonBackupRestoreExecutor.VerifyAndRestore`.
- `beacon/internal/backup/backup.go:33-69` — `BackupProgress{BytesProcessed,TotalBytes,Phase}`, `BackupInterface.SetProgressCallback`.
- `beacon/internal/backup/local.go:111-471` — `reportProgress` call sites.
- `beacon/internal/backup/verification.go:31-118` — `VerifyBackup` (download+SHA256).
- `beacon/internal/server/server.go:351,1541-1544,2048-2101` — `backupProgressWS` WebSocket + `eventBus`.
- `forge/api/internal/http/server.go:1975-1999,2408` — realtime proxy routes (`console|stats|logs` only) + `ws/ticket`.
- `forge/web/lib/api/ws/websocket-manager.ts:1-236` — panel WS manager (used only for console/stats).
- `forge/web/lib/api.ts:933-940` — `connectServerWebSocket` (console/stats/logs only).

**Docs / onboarding**

- `docs/upgrading.md:0-80` — upgrade backup/restore (`pg_dump`).
- `docs/installation.md:334` — compose base list including `postgres-backup`.
- `web/lib/docs.ts:9-303` — backup storage docs + architecture flows.
- `forge/web/components/admin/AdminNodes.tsx:844-938` — node onboarding (DAEMON_NODE_ID/TOKEN).
- `forge/web/app/admin/failover/page.tsx:1-212` — recovery threshold policies.

---

## 3. Reference Baseline — What “Good” Looks Like

### Kopia

| Signal | How emitted | Cadence | Consumer |
|---|---|---|---|
| `HashingFile` → `FinishedHashingFile` per-file | `upload.Progress` | per file | CLI spinner + `UITask` counters |
| `HashedBytes` / `UploadedBytes` / `CachedFile` | counters | streaming | `cli_progress.go:149-161` line: `X hashing, Y hashed (N B), Z cached (M B), uploaded K B` |
| `Error(path, err, isIgnored)` | split fatal vs ignored | streaming | inline `! Ignored error…` / `Error…` + final counts |
| `EstimatedDataSize` → `timetrack.Estimator` | `classic` / `rough` / `adaptive` (threshold 300k) | once + updates | `%.1f%%` + `time left` + free-space estimate |
| `--json` | `jsonOutput` flag | per manifest | `snapshot create --json`, `snapshot verify --json` produce `result` JSON |

Kopia's `CountingUploadProgress.UITaskCounters:312-343` emits **9 counters** as typed `CounterValue`s for the server UI: `Cached Files/Hashed Files/Processed Files`, `Cached/Hashed/Processed Bytes`, `Uploaded Bytes`, `Excluded Files/Directories`, `Errors`, plus live `Estimated Bytes/Files` until final.

### Restic

| Signal | JSON `message_type` | Fields | Cadence |
|---|---|---|---|
| Live status | `status` | `percent_done, total_bytes, bytes_done, total_files, files_done, error_count, seconds_elapsed, seconds_remaining, current_files[]` | throttled `Updater` interval (quiet/json/canUpdateStatus aware) |
| Per-item | `verbose_status` | `action: new|unchanged|modified`, `data_size, data_size_in_repo, metadata_size` | per dir/file |
| Errors | `error` | `during: scan|archival, item, error.message` | per error |
| Excluded | `excluded_item` | `item` | per excluded path |
| Summary | `summary` | `files_new/changed/unmodified, dirs_new/changed/unmodified, data_blobs, tree_blobs, data_added, data_added_packed, total_files_processed, total_bytes_processed, total_duration, backup_start/end, snapshot_id` | once at `Finish` |
| ETA | `seconds_remaining` | `todoBytes / rate` with 1024 B/s floor, gated on `scanFinished` | in `progress.go:70-77` |

Both references support **non-zero `TotalBytes` from scan completion**, **dedup accounting** (`DataSizeInRepo`/`Packed`), and **verbosity-gated detail** (Restic `-vv`, Kopia `--progress`).

---

## 4. Forge Surface Inventory — What Actually Exists

### 4.1 Server-scoped tar path (daily tab)

`forge/web/components/server/backups-view.tsx:43-47` polls with `refetchInterval: 3000` **only if any entry is `pending || running`**, otherwise no polling.

Row rendering (`:99-143`) shows per-backup: `name`, `status.toUpperCase()`, `formatBackupBytes(size)`, `checksum` or `"Checksum not available"` as static text, two dates (`createdAt`/`completedAt`), 4 icon buttons (Download, Lock/Unlock, Restore, Delete). Helper `isUsable:20-21` gates 3 of 4 actions to `status === "completed"`.

No progress bar, no percent, no phase, no ETA, no error drawer, no verify action, no drill action. `actionError:74` merges five mutation errors into one red banner. `"Only completed backups can be downloaded, restored, or deleted."` is the only instructional text.

Advanced drawer (`:182-219`): `Backup Name` (optional), `Ignored Files (comma-separated)` plus a **hard-coded dead card** `Custom storage destinations … are not supported yet. Backups currently use the default node-local storage.` even though the API has S3/GCS/Azure adapters (`handlers_backup_extended.go:477-492`).

### 4.2 Admin 5-tab backup & recovery

`forge/web/app/admin/backups/page.tsx:240-923` polls each query at `30_000` ms. Tabs: Overview (4 metric cards + Storage Providers list + Quick Actions), Configurations, Jobs (`progress:419` rendered only as `"{Math.round(job.progress)}%"`), Artifacts (Verified/Locked pills), Restores (`progress%`). No bytes, no phase, no ETA.

Jobs cancel (`:756` `cancelJobMut` on `running`), artifacts lock/unlock, restores delete. Providers list enumerates `local|s3|gcs|azure|beacon`. Generation of artifacts/jobs ties back to `services/backup`.

### 4.3 Console power-user page

`forge/web/app/console/backups/page.tsx:155-836` is the **most complete** surface, but it lives outside the daily server tab.

- Storage card (`:421-494`) has real tabbed forms (`StorageS3Form`/`GCS`/`Azure`/`Local`) with fields for endpoint/bucket/region/prefix/keys, **but Save merely toasts** `handleSaveStorage:269-272` — `Save S3 config` never calls `POST /admin/backups/storage-providers` (`onSave` is `() => toast + refetch`, no mutation).
- Artifacts table (`:497-602`) shows `storageProvider` icon, `fileSize`, clickable truncated `fileHash` → modal with copy button + `isVerified/isLocked/hashAlgorithm`, manifest JSON viewer. Buttons: Shield (checksum), FileCheck (manifest), Play (drill).
- Retention editor (`:605-628`) drafts `maxBackups/retentionDays/retentionWeeks/retentionMonths/cleanupSchedule/priority`, **but Apply only patches `policies[0]`** (`:274-290`) with a `maxBackups+retentionDays` subset — weekly/monthly/cron/priority are local state lost on submit. Static paragraph claims `Retention is enforced per-policy … and globally via artifact retention`.
- Restore drills table (`:631-706`) shows `progressPercentage` as a 64 px bar + `%` + `currentPhase` + `verificationStatus (passed/failed/pending verify)` + `errorMessage`. Polls restores at `15_000`.
- Policy table (`:334-418`) shows `Interval/Retention/Storage/Options(Compress/Encrypted/Volume)/NextRun/Status`, Lock/Unlock via `lockBackupPolicy`.
- BackupPolicyModal (`:939-1047`) surface an `interval ("0 2 * * * or 24h")`, `storage` selector including `beacon (legacy)`, optional S3/GCS/Azure details that again are **not persisted**.

All three pages fetch overlapping data from different endpoints with different pagination envelopes.

### 4.4 Beacon data-plane progress (exists, unused)

`beacon/internal/backup/backup.go:34-38` defines a lean callback:

```go
type BackupProgress struct { BytesProcessed int64; TotalBytes int64; Phase string }
type ProgressFunc func(progress BackupProgress)
```

`beacon/internal/backup/local.go:111-116` publishes it lazily:

```go
func (l *LocalBackup) reportProgress(bytesProcessed, totalBytes int64, phase string) {
    if fn := l.progress; fn != nil { fn(BackupProgress{bytesProcessed, totalBytes, Phase: phase}) }
}
```

Call sites (`:241,273,411,471`) are:

- `"creating backup"` at `0,0`,
- `"archiving files"` at `0,0`,
- `"completed"` at `info.Size(),info.Size()` once on success,
- `"restoring backup"` → `"completed"` on restore.

No per-file increment, no hashed vs uploaded split, no `CurrentDirectory`. `S3Backup:121-123` just delegates to the same `local.SetProgressCallback`.

`beacon/internal/server/server.go:1541-1544` wires the callback to an in-memory bus per backup creation:

```go
s.backups.SetProgressCallback(func(p backup.BackupProgress) {
    s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
})
```

`server.go:351` registers `GET /servers/{id}/ws/backup` and `server.go:2048-2101` implements `backupProgressWS` with scope check (`ScopeWebsocket|ScopeBackupDownload`) + per-server subscription `ch := s.eventBus.Subscribe(BackupProgressEvent+":"+serverID)` and forwards raw JSON messages over the WebSocket with `pingWebSocket`.

### 4.5 Forge API backup routes

- `handlers_servers.go:1912-1940` `GET /servers/:id/backups` (serverPermission `backup.read`, `page/perPage`).
- `handlers_servers.go:1959-2077` `POST /servers/:id/backups` validates `validBackupName`, checks user cap `CheckUserCanCreateBackup` + server `BackupLimit` + rate-limit via `GetPanelSettings`/`CountRecentBackups`, writes a `pending` row (`UpsertBackup`), then `go func()` with **15-minute timeout** `CreateBackup` → `UpsertBackup` to `failed|completed`. Returns `202` with the pending row. Idempotency via legacy `Get` before `Create` at `server.go:1547`.
- `handlers_servers.go:2104-2168` `POST /servers/:id/backups/restore` (idempotencyKey via `Idempotency-Key` header, dispatches `OperationService.DispatchBackupRestore` when available, else sync `RestoreBackup` with status transitions `restoring → restored|restore_failed`).
- `handlers_servers.go:2170-2224` `GET /servers/:id/backups/verify` (requires `completed`, lists daemon backups, verifies `Size>0 && Checksum!=""` + case-insensitive checksum match; returns `{verified, checksumMatch, dbChecksum, daemonChecksum, daemonSize, dbSize}`) — **not called from `BackupsView`**.
- `handlers_backup_extended.go:40-240` policy CRUD + `/backup/providers`, `/admin/backups/{cleanup,status}`.
- `handlers_backup_extended.go:242-505` `/admin/backups/{configs,jobs,artifacts,restores}` + `storage-providers` + `/restore` + `download`.
- `forge/api/internal/http/server.go:1975-1999` realtime proxy exposes only `stats|logs|console`, and `v1.Get("/servers/:id/ws/{stats,logs,console}", fiberws.New(realtimeProxy(...)))`. **No `ws/backup`.**

### 4.6 Recovery coordinator

`forge/api/internal/services/recovery/backup_restore.go:32-84` `VerifyAndRestore` requires `TargetNodeID && ServerID && SourceBackupName && SourceBackupChecksum && SourceBackupSize>0`. It:

1. Loads `RecoveryRestoreTarget` (nodeURL+token),
2. `ListBackups` on destination, strict equality match on `Name && Status==completed && Size==SourceBackupSize && EqualFold(Checksum)`,
3. **Downloads the entire archive to re-hash SHA-256** (`verifyBackupChecksum:86-113`, 32 KiB loop, full stream),
4. `RestoreBackup(..., truncate=true)`,
5. Journaled ownership: `BeginRecoveryOwnership` → `ProvisionRecoveredServer` → on failure `RollbackRecoveryOwnership` → `CompleteRecoveryOwnership`,
6. Marks backup `restored`.

No progress events emitted from this path either.

---

## 5. Comparison Table — Reference vs Forge (14 rows)

### C01 — Live progress cadence & transport

| | Reference | Forge |
|---|---|---|
| **Kopia** | `cli_progress.go:43,136` 300 ms throttle `outputThrottle.ShouldOutput(progressUpdateInterval)` per `HashedBytes/UploadedBytes` event; spinner `|/-\*` rotates per update. | `BackupsView:47` throttled to **one poll per 3 s** and only while any row is `pending||running`. Otherwise silent. No WS. |
| **Restic** | `progress.go:59-82` dedicated `Updater` goroutine `progress.NewUpdater(interval, …)` + `rateEstimator:73` ETA; `internal/ui/termstatus` live line. | Admin page `30_000` ms, console restores `15_000` ms, server tab `3_000` ms — three unrelated intervals, none aware of file-level events. |
| **Gap** | Streaming with 5-10 Hz granularity → user sees drive activity. | Discrete stale snapshots; a 4-minute backup can show zero movement for 90 % of its runtime, then flip to completed. Appears hung. |

**Evidence:** `reference/backup/kopia/cli/cli_progress.go:126-134,149-191` vs `forge/web/components/server/backups-view.tsx:47` vs `beacon/internal/server/server.go:2048` (unused WS).

---

### C02 — Progress signal granularity

| | Reference | Forge |
|---|---|---|
| **Kopia** | 5 live numbers: `hashedBytes+cachedBytes+uploadedBytes` + `hashedFiles+cachedFiles+inProgressHashing` + `ignored/fatal errors`; plus `Processed Files = hashed+cached` in `UITaskCounters:320-326`. | Beacon `BackupProgress:34` has **2 numbers + 1 string**: `BytesProcessed, TotalBytes, Phase`. Beacon `local.go:111` sets them `0,0` for two of three phases; final `info.Size()` only. |
| **Restic** | Live `total Files/Bytes` + `processed Files/Bytes` + `errors` + `current_files[]` in every `status` event. | `handlers_servers:2028-2076` async goroutine never calls `UpdateBackupJobProgress`; Forge's richer `JobService.Update:283-313` with `bytesProcessed+phase+progressPercentage` is only reachable via the separate `/admin/backups/jobs` stack (not the daily tab). |
| **Gap** | Users can diagnose "hash-bound vs upload-bound". | Server backup tab cannot distinguish scanning/archiving/uploading/failed-at-byte-N. |

---

### C03 — Estimation & ETA

| | Reference | Forge |
|---|---|---|
| **Kopia** | `EstimationParameters{Type: classic|rough|adaptive, AdaptiveThreshold: 300_000}:11-26` + `EstimatedDataSize(fileCount,totalBytes):246-257` → `timetrack.Estimator.Estimate(hashed+cached, estimatedTotal) → PercentComplete + Remaining` rendered `reference/backup/kopia/cli/cli_progress.go:185-188`. | No estimation interface on `BeaconInterface`. `EstimatedDataSize` never emitted; CLI fallback `, estimating…` branch would permanently show for a small backup, but Forge's wire never even exposes `EstimatedBytes` (`forge/api/internal/services/backup/job.go:34`). |
| **Restic** | `progress.go:56-58,70-77` `estimator.recordBytes` per `addProcessed`; `secondsRemaining = todo/rate` gated on `scanFinished`, zeroed below 1024 B/s. | `ConsoleBackupsPage:678-683` renders a bar as `progressPercentage ?? (status==completed?100:0)` — purely state-derived, no rate. `BackupsView` shows no bar at all. |
| **Gap** | Users see `41.2% • 2m left • estimated 4.3 GB`. | Users see `running` with no horizon; abandonment risk and support tickets about "stuck backup". |

---

### C04 — Machine-parseable JSON streaming

| | Reference | Forge |
|---|---|---|
| **Kopia** | `snapshot create --json` / `verify --json` → `jsonIndentedBytes(Result, "  ")` (`command_snapshot_verify.go:94`). `--json` suppresses human stats (`:86`). | `GET /servers/:id/backups` returns `{data,meta,pagination}` with no progress fields; `POST` returns a single `202 {status: pending}`. Per-update JSON over WS exists at beacon but is **unproxied** (`forge/api/internal/http/server.go:1975`). |
| **Restic** | `backup --json` line-delimited `{message_type: status, percent_done, seconds_remaining, current_files} | jq`. Scripted. | `GET /admin/backups/jobs` returns `BackupJob[]` with `progressPercentage` but requires polling to observe. No `message_type` multiplex. No `summary` snapshot with dedup detail. |
| **Gap** | Automation can drive retries/alerts on `--json`. | Operators cannot tail a backup from CLI/API with stable `message_type` guarantees. |

---

### C05 — Error accounting & per-file diagnostics

| | Reference | Forge |
|---|---|---|
| **Kopia** | `Error(path, err, isIgnored)` separates `fatalErrorCount` vs `ignoredErrorCount:110-118`, renders `"X fatal errors (Y ignored)"` + inline per-file `Ignored error when processing "…"`. Counters include `CurrentDirectory, LastErrorPath, LastError` in `Counters:197-201`. | Forge `job.go:315-319` stores a single `ErrorMessage string*` plus `RetryCount/LastRetryAt`. `BackupsView:74` merges 5 mutation `Error`s into one banner `errorText(actionError, "Backup action failed.")` with no path, no ignored vs fatal. |
| **Restic** | `json.go:68-87` streams `{message_type: error, during: scan|archival, item, error.message}` per file, plus aggregate `error_count` in every status tick. Verbose mode fires `verboseUpdate:action`. | `handlers_servers:2051-2057` on daemon failure writes `UpsertBackup{Status: failed}` with no per-file breakdown; UI shows only `status: failed`. |
| **Gap** | "Which files failed, were they ignorable?" unanswerable in Forge. Users cannot fix a `.pteroignore`/permissions issue without logs. |

---

### C06 — Verification UX

| | Reference | Forge |
|---|---|---|
| **Kopia** | `snapshot verify` supports `max-errors, directory-id, file-id, snapshot-ids, sources, --parallel 8, --file-queue-length 20000, --verify-files-percent 0..100`, parallel tree walk `snapshotfs.Verifier:56-99`, JSON stats, expected totals from dir summary `AddToExpectedTotals:199`. | Two disjoint verifiers: `beacon/internal/backup/verification.go:31-78` (`VerifyBackup` re-downloads+SHA256, single-string equality, no percent sampling, no parallelism) and `handlers_servers:2170-2224` (`GET /backups/verify` that **only checks list-equality** without re-hashing). Neither is exposed as a button in `BackupsView`. Admin artifacts show only a static `isVerified` pill + `VerificationAttempts/LastVerifiedAt:51-52`. Console artifacts modal can display checksum but has **no Re-verify button**; the implied verify is the hidden drill (`handleRunDrill:292-307` creates a `POST /admin/backups/restore` with `artifactId`). |
| **Restic** | `restic check --read-data` / `prune --repack-uncompressed` with JSON progress. | `forge/api/internal/services/backup/artifact.go:515-582` `ArtifactService.Verify` does stream SHA-256 but is only invoked from `jobService` post-backup; manual re-verify via admin is absent. |
| **Gap** | Kopia/Restic make "prove this backup is restorable" a first-class action with progress. Forge makes it an easter egg. |

---

### C07 — Checksum presentation & trust

| | Reference | Forge |
|---|---|---|
| **Kopia** | Content-addressed blobs; verification hashes full object store, never truncates hash in logs. | `BackupsView:108` shows `Checksum: ${backup.checksum}` **or** `"Checksum not available"` as unstyled mono text truncated to parent width, no copy, no hashAlgorithm label. `console/backups:147-151` `shortChecksum` truncates `abcdefgh…ijklmnop` with `underline decoration-dotted`. Checksum copy lives only in the hidden modal (`:756-768`). `Admin/backups:804-806` shows `fileHash` raw but no algorithm. |
| **Restic** | Prints `snapshot_id` hex + `SHA-256` per file on demand. | `BackupArtifact.HashAlgorithm:48` exists (`sha256`) but server-tab never shows it; users cannot prove which hash they saw. |
| **Gap** | Copy/paste fidelity matters for incident response. Forge buries the only copy affordance two modals deep on a separate page. |

---

### C08 — Completion summary & deduplication report

| | Reference | Forge |
|---|---|---|
| **Kopia** | Final line includes `hashedBytes/cachedBytes/uploadedBytes` with total, plus dedup from `UITask` if enabled. | `handlers_servers:2059-2076` on completion only writes `Checksum/Size/Status=completed`. Response to user is the original **pending `202`** from 2029 — no final size delivered via the same request; client must re-poll. |
| **Restic** | `summaryOutput:234-253` → `files_new/changed/unmodified, dirs_new/changed/unmodified, data_blobs, tree_blobs, data_added, data_added_packed, total_files_processed, total_bytes_processed, total_duration, backup_start/end`. `data_added_packed` exposes dedup/compression ratio. | Forge `BackupsView` formats bytes via `formatBackupBytes:13-18` (kB/MB only, no dedup math). Admin jobs show `Progress 100%` but no `DurationSeconds` rendered outside the `restores` drill bar, and no `DataSizeInRepo` delta. |
| **Gap** | Operators cannot answer "how much dedup/compression did we get?" or correlate cost. |

---

### C09 — Cancellation & resumability

| | Reference | Forge |
|---|---|---|
| **Kopia** | `--checkpoint-interval` persists incremental snapshots; `upload --parallel N`; cancel via SIGINT with clean `UploadFinished` while flushed files remain visible. | `JobService.Cancel:425-452` cancels only via `beaconClient.CancelTask(taskID)` when `beaconClient != nil` (which is **nil** on the daemonClient path). `BackupsView` has **no Cancel button** for `pending/running` items — buttons are `disabled={!isUsable}` (`:115,139`). Admin jobs table has a Cancel item but only on `/admin/backups` (`:756-757`). Beacon `backupMu` is a single global mutex (`server.go:1540`), so two concurrent backup creates serialize silently; the second appears to hang with `pending` forever if request coalescing not hit. |
| **Restic** | Lock file detects `repository is already locked`; `--no-cache`; resumable `check --read-data-subset`. | Forge async `CreateBackup` uses a **15-minute context** (`handlers_servers:2046`) with no checkpoint resume; partial archives are discarded. |
| **Gap** | No user-visible cancel on the primary page; a stuck backup must be deleted after it finally fails, which risks deleting the wrong generation. |

---

### C10 — Storage adapter visibility & misdirection

| | Reference | Forge |
|---|---|---|
| **Kopia** | Repository exposes `s3/gcs/azure/b2/sftp` in `kopia repository status`; adapter choice visible in every log line. | Server tab card `backups-view.tsx:205-208` is hard-coded: `"Custom storage destinations (S3, GCS, Azure) are not supported yet. Backups currently use the default node-local storage."` — written **after** `handlers_backup_extended.go:477-492` and `storage_s3.go` added S3/GCS/Azure adapters, so statement is stale and misleading. Console page storage card claims `"Forge owns GCS/Azure/S3/Local · Beacon uses S3 path for all"` and `"S3-compatible endpoint covers AWS S3, MinIO, and any S3-compatible storage"` while `Beacon` only ever sets `StorageProvider: beacon` (`beacon/internal/backup/job.go:533,652`). |
| **Restic** | Backend list in `restic backup --help`; every run logs `repository opened (type s3)`. | Admin `Storage Providers` pill list (`forge/web/app/admin/backups/page.tsx:623-636`) can show `s3/gcs/azure/local` when seeded from `backup.RegisteredProviders()`, but no write path from S3/GCS/Azure forms (see C12). |
| **Gap** | Operators on the daily tab incorrectly conclude multi-node recovery is impossible, so they never ask for the shared storage that the backend already supports. |

---

### C11 — Retention & GC feedback

| | Reference | Forge |
|---|---|---|
| **Kopia** | `kopia maintenance` + `snapshot expire` policies with `keepLatest/keepDaily/keepWeekly/keepMonthly` counts and GC progress counters; logs `Deleted N blobs`. | `console/backups:605-628` retention editor collects 6 fields but submits **2** (`maxBackups, retentionDays` onto first policy, `:285-289`); `retentionWeeks/Months/cleanupSchedule/priority` are local fiction. Admin page retention concept is `CleanupExpiredBackups:74-75` + global sweep `POST /admin/backups/cleanup` and artifact `expired` count in `handlers_backup_extended.go:545-546`, but no UI progress bar during sweep. |
| **Restic** | `prune` shows scanning/packing progress with `--json` `message_type: status`. | Lock-gated deletions: `beacon` `HasSpaceAvailable` check vs Forge `BackupLimit` quota vs `maxBackups` policy — three competing quota sources with no reconciliation display. |
| **Gap** | "How far from GC will we delete X?" unanswerable; risk of unbounded `local` filesystem fill on Forge host despite `178_backup_retention.sql` migration existing. |

---

### C12 — Dead controls / blind saves

| Control | Promise | Reality | Reference analogue |
|---|---|---|---|
| Console storage pills **Save S3/GCS/Azure/Local** | `btn Save S3 config` suggests persistence | `handleSaveStorage:269-271` does `toast("Storage config saved for S3") + refetch` — **no POST**. Provider stays `local`. | Kopia `kopia repository connect s3 …` persists endpoint/bucket. |
| Console policy modal S3/GCS/Azure detail inputs | Policies carry per-storage fields inline | Inputs (`s3Endpoint/Bucket`, `gcsBucket`, `azureContainer`) are **dropped on submit** — `submit:966-976` only sends `{interval,maxBackups,retentionDays,storage,compress,encrypted,enabled}`. | Restic `s3:*` flags all reach the backend config. |
| Console `Run restore drill` ▶ on artifacts | "Verify work without truncating live data" | Creates a **real `POST /admin/backups/restore`** (`handleRunDrill:293-301`) with `restoreType: server` + `createBackupBeforeRestore: false` — it exercises `recovery/backup_restore.go` path with live volume attach, not a dry-run sandbox. Banner warns but does not gate. | Kopia `verify --verify-files-percent` downloads without restoring; Restic `check --read-data-subset` reads, not restores. |
| `BackupsView` Lock on create | Checkbox `Lock backup on creation` (`:210-216`) | Threaded as `is_locked: lockOnCreate` to `POST /servers/:id/backups` but never reflected as a pill until re-poll; no confirmation that lock survived the async completion. | Kopia pins (`--pin`) appear in `snapshot list` immediately. |

These fake-success writes are classic "appears dead" signals: the UX claims action while the wire did nothing.

---

### C13 — Transport that exists but is unbridged (the core "appears dead" finding)

| Layer | Claim | Evidence |
|---|---|---|
| Beacon | `GET /servers/{id}/ws/backup` WS streaming `BackupProgress` exists, authenticated, per-server. | `beacon/internal/server/server.go:351` registration + `server.go:2048-2101` implementation, `eventBus.Publish:1542`. |
| Forge API | Real-time proxy forwards `console/stats/logs` | `forge/api/internal/http/server.go:1975-1999` only proxies `stats/logs/console`, no `backup`. `IssueWSTicket:2408` issues WS tickets, but only for those three streams. |
| Web | `WebSocketManager:1-236` supports factory functions for any WS | Used at `console-view.tsx:146,192` for `console/stats` via `connectServerWebSocket` targeting `/servers/{id}/ws/{console|stats}`; `lib/api.ts:933-940` helper hardcodes `stream: "console"|"stats"|"logs"` union, no `backup`. No `forge/web` file `grep -rn backup.*ws` matches. |

**Effect:** Even though beacon emits `reportProgress` (however coarse), **no packet ever reaches the browser**. The user perceives the feature as dead when the engineering is 90 % done.

---

### C14 — Status lifecycle clarity

| Reference | Forge |
|---|---|
| Kopia spinner phase cycles through `hashing → hashed → cached → uploaded` in one line. Restic's `progress` maps each file to `ActionFileNew/Unchanged/Modified` + `ActionDirNew/Unchanged/Modified` (`progress.go:130-143`). | Forge fractures one concept across four vocabularies: `beacon BackupInfo.Status` = `completed`, `store Backup.Status` = `pending|running|completed|failed|restoring|restore_failed|restored`, `JobService BackupStatus` = `pending|running|completed|failed|cancelled`, `RestoreService status string` = `pending|running|completed|failed`. `BackupsView:isUsable:20` collapses all non-`completed` into "Available after completion" (`:115`), hiding whether a backup is queued, running, restoring, or poisoned. Admin Overview cards roll up `running/pending/failed` separately for jobs (`:599-603`) while the server tab collapses them. `verify` adds `verified/checksumMatch` as orthogonal booleans not in any status enum. |

Users cannot answer "is it running right now, or waiting for a daemon lock?" without reading DB rows.

---

## 6. Invisible Capabilities (What Works but Never Shows Up)

1. **Cross-node verified restore** (`recovery/backup_restore.go:47-83`) with size+checksum match and journaled ownership — reachable only via failover/migration internals, not from `BackupsView`.
2. **Artifact checksum + manifest browsing** (`console/backups:557-575` checksum modal, `789-834` manifest viewer) — discoverable only on `/console/backups`, not where 90 % of users interact.
3. **Encryption-at-rest keys** (`backup/encryption.go:91` Kopia-model comment, `handlers_backup_extended.go:89-90` `EncryptionKey`, `artifact.go:47` `EncryptionAlgorithm`) — no rotation/verification badge in any UI.
4. **Ignored files / .pteroignore** (`ignore.LoadIgnoreReader` wired in `server.go:1530` + `ignored_files` payload) — exposed only in the advanced drawer text field; no template, no preview, no validation feedback.
5. **`Idempotency-Key` for restores** (`handlers_servers:2136-2141`) — documented nowhere in the UI; retry storms still possible.
6. **Rate-limit settings** (`handlers_servers:1975-1979` `BackupRateLimitEnabled/Count/WindowMinutes` via `GetPanelSettings`) — no panel UI to configure per-node caps.
7. **User-level backup caps** (`handlers_servers:1966-1972` `CheckUserCanCreateBackup`) — distinct from `BackupLimit` per server, but the quota banner (`:90`) only shows `slots used` for the server.
8. **`GET /servers/:id/backups/verify` + `GET /servers/:id/backups/storage/download`** — zero call sites in `forge/web`.

---

## 7. Confusing / Dead Controls Audit

| Control | Location | Confusion | Severity |
|---|---|---|---|
| Storage dead card in advanced drawer | `backups-view.tsx:205-208` | Says custom storage unsupported while console page claims S3/GCS/Azure ready | **High** — misdirects roadmap trust |
| `Save S3/GCS/Azure/Local config` toast-only | `console/backups:269-272,868-869,893-894,914-915,931-932` | Feels persistent, isn't | **High** |
| Retention weeks/months/cron/priority fields | `console/backups:605-628` | Only two fields applied to one policy; rest silently dropped | **High** |
| Backup limit banner | `backups-view.tsx:90` | Shows `"{total} backups created; no quota was provided by the API."` when limit is `null`, inviting operators to think caps are absent while `CheckUserCanCreateBackup` can still 422 | **Medium** |
| Lock on create | `backups-view.tsx:210-216` | Async completion may race, lock bit not shown until re-poll; no confirmation | **Medium** |
| Download button | `backups-view.tsx:115` | `isUsable` gating disables for `restoring`/`restored` archives that *are* downloadable; user thinks download broken | **Medium** |
| `Restoring backup…` chip | `backups-view.tsx:92` | Bound to `restoreMutation.isPending` local mutation state, not actual beacon restore phase; disappears on refetch/page nav | **Low** |
| `Transfer` / `Failover` tabs | server nav + `forge/web/app/admin/failover/page.tsx` | Recovery actions visible without prerequisite shared storage preflight — evacuation renders as button but silently fails `target daemon cannot verify access to the planned backup archive` unless S3 is already shared (`recovery/backup_restore.go:83`) | **High** |

---

## 8. Logic Findings (3 × Real Issues)

### L01 — `L-DR-BACKUP-01` — Async `pending` record never resolves if daemon still running past 15 min → zombie `running` + no progress → perceived dead backup

**Location:** `forge/api/internal/http/handlers_servers.go:2044-2076`

```go
go func() {
    backupCtx, backupCancel := context.WithTimeout(context.Background(), 15*time.Minute)
    defer backupCancel()
    backup, daemonErr := cfg.Daemon.CreateBackup(backupCtx, ...)
    persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer persistCancel()
    if daemonErr != nil {
        if _, upsertErr := cfg.Store.UpsertBackup(persistCtx, target.ServerID, store.UpsertBackupRequest{
            UUID: stored.UUID, Name: stored.Name, Status: "failed", CompletedAt: &now,
        }, actorID); upsertErr != nil { slog.Error(...) }
        return
    }
    // Upsert completed
}()
return c.Status(fiber.StatusAccepted).JSON(stored) // returns pending 202 immediately
```

**Contract violation:** `POST /servers/:id/backups` returns a `StatusField: pending` row that the **same goroutine** is responsible for flipping to `completed|failed`. If `CreateBackup` hangs (S3 stall, large world > 15 min, beacon mutex busy) the `backupCtx` deadline fires, `daemonErr == context.DeadlineExceeded`, the persist writes `failed`, but the daemon goroutine **continues locally** and will succeed minutes later without a second persist. The DB row now reads `failed` while a valid `.zip` sits on disk. Conversely, if the daemon goroutine crashes after deadline (beacon restarted), the row stays `pending` forever — no sweeper enumerates stuck `pending` rows and flips them after, e.g., `updatedAt < now - 18m`.

**Evidence of stall without feedback:** the poller `BackupsView:47` shows `pending` for exactly 15 minutes, then flips to `failed` with no `ErrorMessage` (the 10 s persist context may itself expire if DB is contended). No `ProgressPercentage/BytesProcessed/Phase` exists on this path to explain the stall. Beacon's `BackupProgress` WS is not forward-proxied, so no ETA can save the operator. The sibling system `services/backup JobService.Execute:327-423` has retry loops (`RetryCount/MaxRetries`, `ClaimBackupJobForExecution` CAS) — **none of that logic runs on the tar-path**; it is a separate blind goroutine per request.

**Impact:** Users retry, creating duplicate `backup-%Y%m%dT%H%M%SZ` rows that each race the single `backupMu` (`server.go:1540`). On a 30 GB world with 50k files, the naive non-checkpoint path legitimately exceeds 15 minutes; users observe "backup failed" with no diagnostic, then "target daemon cannot verify access" on the subsequent restore.

**Suggested fix:** Enforce `Store.MarkBackupStatus` heartbeat (every `reports progress 30 s`) on a leased row, plus a sweeper (`Cron("0 * * * *")`) marking `pending` older than `BackupTimeout+5m` as `failed` with `error: daemon timeout`, and surface `ErrorMessage` in `BackupsView` (currently 5 merged errors, no sticky error per backup row).

---

### L02 — `L-UX-BACKUP-02` — `backupProgressWS` is a complete dead path: beacon publishes, Forge never subscribes, Web never connects

**Locations:**  
beacon publish `beacon/internal/server/server.go:1541-1544` →  
beacon WS `server.go:2048-2101` (`BackupProgressEvent:61`, `eventBus.Subscribe:2085`) →  
Forge proxy omission `forge/api/internal/http/server.go:1975-1999` (no `ws/backup` route) →  
Web omission `forge/web/lib/api.ts:933-940`, `forge/web/components/server/console-view.tsx:150,196` (no `backup` ticket type) →  
grep zero `forge/web --include=*.ts --include=*.tsx backup.*ws|backupProgress`.

**Logical consequence:** `SetProgressCallback` is set **inside `createBackup`** on every request (`server.go:1542`), but overwritten atomically per request (`progress` is a plain function pointer on the adapter with no per-server slot — race if two servers on the same beacon back up concurrently). Even if correctly routed, beacon's increments are `0,0` until done (`local.go:241,273`), so a future WS proxy would forward `{"bytesProcessed":0,"totalBytes":0,"phase":"archiving files"}` forever — integration appears alive but delivers no value. This is worse than not implementing WS at all, because incident responders will assume the WS carries byte-level data that does not exist, and will debug the proxy instead of the missing `TotalBytes` estimation.

**Impact:** Controls status check "does backup appear dead?" = **yes** by construction. Test harness watching browser console will see zero WS traffic for backup, while `console`/`stats` streams are active.

---

### L03 — `L-DR-UX-03` — Storage & retention editors are no-op mutations: data the user enters is silently dropped

**Locations:**

- Console Save: `forge/web/app/console/backups/page.tsx:269-272` — `handleSaveStorage` toast-only; `Storage{S3,GCS,Azure,Local}Form:841-935` UI captures `endpoint/bucket/region/keys/prefix` but `onSave` prop is `() => void providersQuery.refetch()` — **no `POST /admin/backups/storage-providers`**.
- Policy detail loss: `BackupPolicyModal:966-976` gathers `interval/maxBackups/retentionDays/storage/compress/encrypted/enabled`; sibling fields `s3Endpoint/Bucket/gcsBucket/azureContainer` are rendered but excluded from `onSave` payload.
- Retention: `retentionDraft:167-174` holds `maxBackups/retentionDays/retentionWeeks/retentionMonths/cleanupSchedule/priority`; `handleApplyRetention:274-290` only forwards `maxBackups/retentionDays` onto `policies[0]` via `updateMut`, ignores 4 fields + applies to the wrong policy when >1 exists (index-0 heuristic).

**Logic error type:** optimistic UI success without backend effect — the page's toasts claim durability while the state is ephemeral local React state. Reloading the tab proves loss, but operators performing retention hardening will not reload before evaluating disaster readiness.

**Impact:** Pre-incident hardening checklists (e.g., "set `--keep-weekly 4 --keep-monthly 6` analogous to Kopia's retention") **appear satisfied in the UI** while the server remains on defaults (`maxBackups=7, retentionDays=30`). Subsequent `CleanupExpiredBackups` will delete more aggressively than the operator intended, or not at all, depending on defaults.

---

### L04 — `L-SEC-BACKUP-04` (bonus third+1) — Verification re-downloads the full archive over a 5-minute bounded context with no streaming verification path

**Locations:** `forge/api/internal/http/handlers_servers.go:2191-2194` `verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Minute)` + `beacon/internal/backup/verification.go:39-65` `io.Copy(hasher, reader)` in one pass + `forge/api/internal/services/recovery/backup_restore.go:86-113` second full-download SHA-256.

**Issue:** A 20 GB backup verification requires downloading 20 GB within 5 minutes over the beacon↔panel link. Failure manifests as `failed to list backups on daemon: context deadline exceeded`, which the UI would render (if it ever called verify) as "backup not verified" — indistinguishable from corruption. Reference Kopia verifies with **percent sampling** (`--verify-files-percent`) and chunk-level hashes without full download; Restic verifies via pack index + optional subset reads.

No incremental/streaming verify with progress exists despite `VerifyBackup` allocating a sha256 hasher that could report `BackupProgress` per 32 KiB read — that hook is unused.

---

## 9. Disaster Recovery & Onboarding Interplay

### Onboarding (`AdminNodes.tsx:844-938`)

- Supplies `DAEMON_NODE_ID/TOKEN`, `PANEL_API_URL`, `DAEMON_BACKUP_DIR=/srv/game-panel/servers/.beacon/backups`, but **no storage preflight**. `docs/installation.md:334` lists `postgres-backup` sidecar but not `S3_BUCKET` envs; `web/lib/docs.ts:181` backup table only lists `POSTGRES_BACKUP_HOST_DIR` vs `S3_*` without stating that evacuation requires *identical* `S3_*` on every node. A single-node install passes onboarding green, then fails recovery (`recovery/backup_restore.go:83`) because `target daemon cannot verify access`.

### Recovery coordinator vs user journey

- Coordinator is **invoked only by failover/migration services**, not by the backup tabs. An operator on `BackupsView` has no "Copy backup to shared storage" or "Validate this backup on node B" action; the console `drill ▶` is page-remote (`/console/backups` vs `/console/servers/:id/backups`) and uses the wrong `restoreType: server` for an app-level artifact.
- Failover page (`/admin/failover:62-97` `recordFailure`) lets operators **manually tick** failure counters to trip `maxFailures` — a recovery design antipattern: no linkage to actual backup freshness, and thresholds are evaluated against wall-clock windows rather than verified backup age.
- Ownership journaling (`BeginRecoveryOwnership/ProvisionRecoveredServer/RollbackRecoveryOwnership:64-68`) on restore failure is invisible in any backup status timeline; the operator sees `failed` without knowing a rollback artifact `RollbackArtifactID` was created and could be re-restored.

### Docs

- `docs/upgrading.md:0` frames "backup" primarily as **DB dumps** (`pg_dump --format=custom`), while `web/lib/docs.ts:155` lumps workload restores as "Beacon reads workload data → backup adapter → local or S3-compatible repository". No doc explains the 15-minute pending horizon, polling cadence, or how to interpret `pending/running/restoring/restore_failed`. The docs shell adds a Backups link (`web/components/docs/docs-shell.tsx:72`) that routes to the isolated docs app, not the panel's backup UI.

---

## 10. `forge/web` API Layer Duplication Note

`forge/web/lib/api/backup.ts:17-27` (`BackupPolicy` with `interval/maxBackups/retentionDays/storage/compress: boolean/encryptionKey`) and `forge/web/lib/api/console-backups.ts:3-23` (`BackupPolicy` with `volumeBackup/isLocked/nextRunAt/appId/databaseId/volumeBackup: boolean`) are **two coexisting policy types** for the same entity served from different endpoints with overlapping paths (`/servers/:id/backups/policies`). The admin 5-tab page internally re-defines a third type (`BackupConfiguration:13-35` with `backupType/cronExpression/storageProvider`). None of them carry the `BackupJob.ProgressPercentage` channel to the daily tab.

---

## 11. Recommendations (prioritized)

1. **Bridge the WS.** Add `GET /api/v1/servers/:id/ws/backup` proxy in `forge/api/internal/http/server.go:1975` (`fiberws.New(realtimeProxy(..., "backup"))`), expose `backup` in `forge/web/lib/api.ts:933` ticket helper, and subscribe from `BackupsView` via `WebSocketManager`. Even at `0,0` today, wiring it surfaces the gap.

2. **Make `TotalBytes` truthful.** Have beacon `scanStats` capture `TotalBytes` via `fs.Walk` (or expose `upload.EstimationParameters` analogue) before archiving, and invoke `reportProgress` per N files/32 KiB (reuse `HashedBytes` hook). Propagate it through `Unblock:CreateBackup` stream, not only at completion.

3. **Eliminate the hard-coded dead card.** Replace `backups-view.tsx:205-208` with provider pill + link to `Console → Storage configuration` when `GET /backup/providers` returns >1.

4. **Fix no-op saves.** Wire `StorageS3Form.onSave` → `POST /admin/backups/storage-providers`, and persist modal S3/GCS/Azure detail payloads (or remove the inline fields). Make retention's `retentionWeeks/Months/cleanupSchedule/priority` hit a real `PUT /admin/backups/retention` or delete the inputs.

5. **Surface verification.** Add a per-row Verify button in `BackupsView` hitting `GET /servers/:id/backups/verify?name=…`, show `checksumMatch/daemonSize/verified` as a pill, and wire admin's `VerifyBackupArtifact` as an `Artifacts` row action.

6. **Cancel & sweep.** Add a Cancel action on `BackupsView` rows with `status===pending||running` (`POST /servers/:id/backups/:name/cancel` or reuse `Delete` with `BeaconTaskID`), plus a sweeper job that marks `pending` older than `18m` → `failed (daemon timeout)`.

7. **Retention correctness.** Collapse console retention to `{maxBackups,retentionDays}` OR implement real weekly/monthly pruning in `storage.retention` and document it; either way remove the 4 decoy fields if the backend does not honor them.

8. **Recovery preflight badge.** On `/admin/failover` and `BackupsView`, show `sharedStorage: green|red` by probing `ListBackups` reachability to at least two nodes, and grey out Evacuate when unreachable with copy linking to onboarding storage docs.

---

## 12. How to Verify the "Appears Dead" Claim

1. `grep -rn "backup.*ws\|ws.*backup" forge/web --include='*.ts' --include='*.tsx'` → **0 hits** (expected >0).
2. Open `BackupsView`, start a backup, observe Network panel: only `GET /servers/:id/backups` every 3 s, no WS `:101 — Switching Protocols`; payload bytes remain 0 until completion.
3. `curl -s -H "Authorization: Bearer $tok" http://127.0.0.1:8080/api/v1/servers/$id/backups | jq .data[0]` → `status: pending`, no `bytesProcessed/totalBytes/phase`.
4. Trigger beacon `GET /servers/{id}/ws/backup` directly against beacon's TLS-exposed `9090` with a websocket token → stream yields at most 3 messages (`creating → archiving → completed`) with zero-byte intermediates, confirming `reportProgress` granularity issue.

---

## 13. Appendix — Line-level Traceability

- Reference spinner cadence: `reference/backup/kopia/cli/cli_progress.go:42,136,185`
- Reference progress interface & counters: `reference/backup/kopia/snapshot/upload/upload_progress.go:32-83,168-202,204-309`
- Reference JSON status/ETA/summary: `reference/backup/restic/internal/ui/backup/json.go:42-64,199-253`
- Reference rate estimator: `reference/backup/restic/internal/ui/backup/progress.go:52-84`
- Forge daily tab & gating: `forge/web/components/server/backups-view.tsx:13-18,20-21,43-47,74,90,99-143,205-208,210-216`
- Forge console storage fake save: `forge/web/app/console/backups/page.tsx:269-272,419,605,756`
- Forge admin progress rendering: `forge/web/app/admin/backups/page.tsx:42,83,419-420,746,865`
- Forge server drill: `forge/web/app/console/backups/page.tsx:292-307`
- Beacon progress shape: `beacon/internal/backup/backup.go:33-41,68`
- Beacon coarse report: `beacon/internal/backup/local.go:111-116,241,273,411,471`
- Beacon WS bus: `beacon/internal/server/server.go:61,1541-1544,351,2048-2101`
- Forge proxy missing backup: `forge/api/internal/http/server.go:1975-1999`
- Forge tar-path async 202: `forge/api/internal/http/handlers_servers.go:2036-2077`
- Web WS helper limited union: `forge/web/lib/api.ts:933-940`
- Pending zombie + sweeper absence: `forge/api/internal/http/handlers_servers.go:2044-2076` / no cron sweeper
- Stale storage card duplicate adapters: `forge/api/internal/services/backup/storage_s3.go` + `handlers_backup_extended.go:477` vs `backups-view:205`
- Recovery double-read SHA256: `forge/api/internal/services/recovery/backup_restore.go:86-113`

---

*Audited against commit window contemporary with Phase 03 kickoff; reference submodules pinned via `.freebuff/worktrees/` mirrors.*
