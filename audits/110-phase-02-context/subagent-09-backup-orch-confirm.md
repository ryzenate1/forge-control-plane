# Subagent 09 — Backup/Storage + Orchestration/Queue Cross-Domain Confirm (Phase 02 Agent 09/10)

**Focus:** Confirm Backup/Storage + Orchestration/Queue implementations against LIVE code
**Date:** 2026-08-24
**Mode:** read-only confirm — no product code modified
**Prior syntheses reconciled:** `audits/phase-03/synthesis.md` (18 findings) + `audits/final-parity/subagent-08-backup-storage.md` (20 rows, BKP-08-01..09) + `audits/final-parity/subagent-09-orchestration-queue.md` (19 rows) + `audits/reverification/subagent-16-backup-storage.md` + `subagent-17-placement-scheduling.md` + `subagent-18-queue-events.md` + `subagent-19-runtime-clustering.md`

**Verdict: ALL 18 PROMPTED SIGNALS STILL BROKEN — cross-domain re-verified at HEAD with file:line proof. No remediation landed.**

---

## Methodology

Every `file:line` below was `Read` against current `HEAD` (2026-08-24). `git diff HEAD --stat` on relevant paths shows only non-semantic diffs (slog warn wrap, whitespace, install-scope hardening) — **zero semantic fixes** for any of the 18 items. Where prior audits diverge in counting, findings are mapped to show de-duplication (e.g. phase-03 F-B-01/F-B-04 → BKP-08-01/03 → BKP-RV16-01 is one bug).

Live surfaces inspected:
- `forge/api/internal/services/backup/encryption.go`
- `beacon/internal/backup/{local.go:90,275, retention.go:61, s3.go, scheduler.go, verification.go, store.go}`
- `beacon/internal/server/server.go:352,1542,2155`
- `forge/api/internal/http/{realtime.go:124, server.go:1995}, forge/api/internal/store/store_backups.go:240`
- `forge/api/internal/services/scheduler/service.go:212,317,714`
- `forge/api/internal/placement/{engine.go:38, constraints.go:59, strategy.go:137}`
- `forge/api/internal/services/replicamanager/service.go:706,883`
- `forge/api/internal/services/queue/{store.go:113, queue.go:247, periodic.go:142}`
- `forge/api/internal/services/operation/service.go:288`
- `forge/api/internal/eventstore/outbox.go:39`, `forge/api/queue/leader.go:23`, `forge/api/internal/services/fencing/fencing.go:22`, `forge/api/internal/store/store.go:132` (TunnelIP)

---

## 1. Backup/Storage — 8 Signals (ALL STILL BROKEN)

### B-01 — `encryption.go:92 nil salt/AAD` — **STILL BROKEN — P0 transplant forgery**

**Ref:** phase-03 F-B-01/F-B-04, BKP-08-01/BKP-08-03, BKP-RV16-01

**LIVE proof:**
- `forge/api/internal/services/backup/encryption.go:18` `purposeEncryptionKey = "gamepanel-backup-encryption"` — single global purpose string
- `encryption.go:92-96` `deriveEncryptionKey(masterKey, purpose)` → `hkdf.Key(sha256.New, masterKey, nil, purpose, 32)` — **salt `nil`** third arg, no per-server/per-backup domain separation
- `encryption.go:119` `gcm.Seal(nil, nonce, data, nil)` — **AAD `nil`** fourth arg; `143` `gcm.Open(nil, nonce, ciphertext, nil)` same; `212-216` `encryptWithKey` identical `nil` AAD
- `encryption.go:26` `GetEncryptionKeyFromEnv()` bypasses `forge/api/internal/secrets/keyring.go:75` envelope `forge:v1:<keyID>:<b64(nonce||seal)>` which *does* use AAD/keyID — service path never calls keyring
- `beacon/internal/backup/local.go:238` `Create` writes **plaintext zip** — zero crypto call; `s3.go:126` `S3Backup.Create` stages via `local.Create` then `PutObject` without encrypting — `s3.go:278` `os.Open→manager.Uploader.Upload` streams plaintext + `Metadata: {sha256: checksum}` unkeyed (`s3.go:27` `checksumMetadataKey`)
- `beacon/local.go:787` `writeMetadata` plain JSON `.metadata.json` (`Chmod 0600` but body unkeyed); `local.go:757-759` `readOrCreateMetadata` returns `metadata.Checksum` verbatim when sidecar exists — **no rehash** — tamper bypass; `verification.go:66` `EqualFold(actual, info.Checksum)` trusts it; `s3.go:318-368` validates `expected=checksumFromMetadata(result.Metadata)` before staging but metadata itself is attacker-writable S3 user-metadata

**Effect:** Attacker with write to local FS or shared S3 bucket (panel-global creds `048_s3_backup_config.sql`) overwrites `backup.zip`+sidecar consistently → `VerifyBackup:66` passes. Even without sidecar tamper, `nil` AAD means ciphertext for `server-A/backup-20260101T000000Z.zip` validates identically under `server-B` if name spoofed → silent cross-tenant restore of wrong-server data. Nonce stored separately as hex `store.go:909 nonce` leaks randomness, collision reuses GCM keystream catastrophically. After master rotation old ciphertext undecryptable (no envelope/keyID header).

**Still broken delta:** `git diff HEAD -- encryption.go beacon/local.go beacon/*backup*` = zero diff; `encryption.go:92,119` verbatim 2026-08-24 read above.

---

### B-02 — `encryption.go:150 io.ReadAll OOM` — **STILL BROKEN — P1 availability/DoS**

**Ref:** phase-03 F-B-02/L2, BKP-08-02, BKP-RV16-02

**LIVE proof:**
- `forge/api/internal/services/backup/encryption.go:150-176` `EncryptReader(r, key)` → `158 input, _ := io.ReadAll(r)` → `162 encryptWithKey(input, derivedKey)` → `166 io.Pipe` replay of already-buffered ciphertext — **buffers entire backup before seal**
- `encryption.go:178-204` `DecryptReader` identical `186 io.ReadAll(r)` → `decryptWithKey` → Pipe replay
- `forge/api/internal/services/backup/service.go:303` `dataBytes, _ := io.ReadAll(data)` before `316 adapter.Upload(ctx, backupPath, dataBytes)` — whole post-compress/post-encrypt archive in heap even though `StorageAdapter` has streaming `UploadStream/DownloadStream` at `storage.go:35` (unused on encrypted path)
- `service.go:312` `nonceHex = hex(data[:12])` leaks nonce into `backups.nonce` DB hex copy after buffering
- `artifact.go:591` `Download` (`data, _ := adapter.Download` → `bytesReader` O(size)) + `877 downloadToTempFile` (whole-file `os.WriteFile`) — same doubling; `compression.go:63` `CompressReader` pipes `gzip/zstd` but immediate `ReadAll` drains buffered pipe then second `ReadAll` copies again → 2× archive + hex nonce copy

**Beacon contrast:** `beacon/internal/backup/s3.go:127-128` `local.Create→272 os.Open→manager.Uploader Upload` streams correctly (3-attempt backoff `139-158`) but API service path clobbers it by buffering first — beacon S3 path is durable, service path is RAM-only.

**Effect:** World 20 GiB → API RAM ≥20 GiB (+ zstd + nonce copy) → OOM-kills `api` pods (default 1 GiB limit in `infra/compose.yml`). Vulnerability window: manual uploads + DB-volume backups via `service.go:271 UploadBackupWithOptions` + any `FORGE_MASTER_KEY` rotation re-encrypt. Test `encryption_test.go:146 TestEncryptLargeData` uses 100 KiB fixture — never catches.

---

### B-03 — `retention.go:61 AND` — **STILL BROKEN — P1 data-loss (semantic inversion)**

**Ref:** phase-03 F-B-05/L3, BKP-08-04, BKP-RV16-03

**LIVE proof:**
- `beacon/internal/backup/retention.go:60-62` comment `// Intersection: a backup must satisfy ALL active rules to be kept.` — explicit AND contract
- `retention.go:64-73` Rule 1 `MaxAge`: `if now.Sub(CompletedAt) < MaxAge { keep[b.ID]=true }` else absent; `70-73` else-branch seeds `keep=all` when `MaxAge==0`
- `retention.go:79-101` Rule 2 `KeepDaily/Weekly/Monthly`: loops `0-24h / 24-168h / 168-720h` and **re-adds** `keep[b.ID]=true` even for backups rejected by `MaxAge` — bypasses age gate, then
- `retention.go:104-118` Rule 3 `MaxBackups`: builds `maxKeep` of newest N, then **intersects** `delete(keep, id)` if `!maxKeep[id]` — pure AND with Rules 1+2; `122 keep[backups[0].ID]=true` safety rail hides bug in smoke tests
- `forge/api/internal/services/backup/service.go:743-768` `EnforceRetentionPolicy:768 keep = isLocked || (withinCount && withinAge)` — **pure AND** (`&&`), both must hold
- `artifact.go:711-738` `ApplyRetentionPolicy:738 (!exceedsCount && !exceedsAge)` — De Morgan equivalent AND but beacon diverges with hybrid
- `forge/api/internal/store/store_backup_policies.go:301` `ListExpiredBackups` hard-codes `30 days` ignoring per-policy `RetentionDays`
- `worker.go:401-422` `enforceRetentionBeforeBackup:422 overLimit = count - MaxBackups +1` **count-only** pre-delete ignoring age, then calls `EnforceRetentionPolicy` AND — double enforcement deletes more than either alone; `ListBackups 1,1000` + `433 sort.SliceStable oldest-first` without `FOR UPDATE SKIP LOCKED` → races

**Reference:** Kopia `retention_policy.go` keepLatest/keepDaily/keepWeekly/keepMonthly **union OR** (keep if *any* bucket keeps, locked exempt); Restic `policy.go:ApplyPolicy` union → Forge explicitly opposite.

**Effect:** Policy `MaxBackups=10, RetentionDays=7` with 20 backups aged 10d — Restic/Kopia keeps ~10 (union); Forge AND keeps 0 unlocked save safety rail single newest → 9 days history unexpectedly discarded. Conversely beacon `KeepDaily` can resurrect `>MaxAge` → retention bypass. `Worker` race double-counts `cleaned++` metric, second `RowsAffected==0` swallowed.

---

### B-04 — `store_backups.go:240 SQL-only` — **STILL BROKEN — P1 orphan leak + racy prune**

**Ref:** phase-03 F-B-06/L4, BKP-08-05, BKP-RV16-04

**LIVE proof:**
- `forge/api/internal/store/store_backups.go:240-259` `CleanupOldBackups(ctx, retentionDays, autoCleanup)` → `DELETE FROM backups WHERE is_locked=FALSE AND created_at < now() - interval '1 day'*$1 AND status='completed'` — **single SQL, no advisory lock, no S3 `RemoveObject`**, DB-only
- `store_backups.go:263-302` `CleanupOldBackupsForServer:277-295` combines `created_at < now()-interval` OR `uuid IN (SELECT uuid ... ORDER BY created_at ASC LIMIT GREATEST(0, COUNT - $3))` — also **DB-only**, not deterministic under concurrent inserts, can delete just-completed newest backup before `FailStaleBackups:348` finalizes it; `subselect` `COUNT(*)` captures concurrent `UpsertBackup:54 ON CONFLICT` before completion
- `service.go:689` `CleanupExpiredBackups` / `743 EnforceRetentionPolicy` loop `DeleteBackupFromStorage` then `DeleteBackup` without `SELECT … FOR UPDATE SKIP LOCKED` — correct per-row but not sole caller; `worker.go:406` `enforceRetentionBeforeBackup` iterates oldest-first without Tx
- `store/store.go:41-55` `pg_advisory_lock` used only for migrations (`migrationAdvisoryLockID`), never in backup prune path (`049_backup_locking.sql` adds `is_locked` column but no `pg_advisory_lock`); `s3.go:163` `List` does `ListObjectsV2` + per-key `HeadObject:187` N+1 to re-derive checksum — still serves S3 orphans
- `beacon/internal/backup/scheduler.go:60` + `store.go:30` `Store{Create/Get/List/UpdateStatus/Delete}` on SQLite only sweeps `backup_jobs`, not storage; `store_admin_backups.go:256` soft-deletes `status='deleted'` never deletes storage

**Effect:** Two replicas each compute `overLimit` from same `ListBackups` snapshot and race to `DeleteBackupFromStorage` same key (S3 idempotent, DB second swallowed) → thundering herd, metric double-count, non-atomic. `CleanupOldBackups` orphans S3 objects forever; `S3Backup.List:163` still lists via S3 → panel vs beacon count drift; `restic prune` requires exclusive `lock.go:47` and aborts on any non-exclusive lock — Forge has no gating, can delete pack still holding live blob (if packing existed) or leave orphans uncollected. Long-run orphan billing growth.

---

### B-05 — `local.go:275 .partial` — **STILL BROKEN — P1 GC orphan leak (no index)**

**Ref:** phase-03 F-B-07/L2, BKP-08-05 scope, BKP-RV16-05

**LIVE proof:**
- `beacon/internal/backup/local.go:275` `os.OpenFile(backupPath+".partial", O_CREATE|O_EXCL|O_WRONLY, 0600)` → `391 Rename(partial→backupPath)` after `385 temp.Sync()` — **only committed path is indexed**; `281-286 defer { temp.Close(); if !committed { Remove(partial) } }` cleans only graceful close, not crash
- `local.go:546` `MkdirTemp(parent, "."+base+".restore-")` + `572 rollback := "."+base+".rollback-"+nanos` + `896 journalPath` + `s3.go:296 CreateTemp(dir, ".s3-download-*.zip")` + `301 cleanup { Remove(file)+Remove(.metadata.json)}` — all staging under daemon-owned `backupRoot`/`serverRoot/.metadata-*`
- **GC gap:** `beacon/internal/backup/scheduler.go:15 Scheduler{gocron.Scheduler, jobs map[*gocron.Job]}` + `store.go:30 Store on SQLite` only sweeps `backup_jobs` table (`scheduler.go:60`); `store_backups.go:242` prune is SQL sweep not index-backed; `DeleteBackupArtifactRecord:256` soft-deletes `status='deleted'` never deletes storage; **no** nightly `ListObjectsV2 vs SELECT uuid` reconciliation like `restic check --read-data` / `kopia content verify`
- `local.go:415` `List(ReadDir)` filters `validBackupName(entry.Name())` → `.partial` excluded → **invisible**; `s3.go:184` `strings.Contains(name,"/") || !validBackupName` same — `.s3-download-*.zip` / `.partial` never listed → no GC can find them; `store_backups.go:18` `ORDER BY created_at DESC` / `ReadDir` ordering hides orphan until manual FS inspection

**Effect:** Crash after staging but before `391 Rename` leaves `.partial` forever; next `Create` same `name` gets `exists` → 409/500, retry must pick new name (control-plane mints `sanitizeBackupName:1539` but beacon Create idempotency `1542-1553` only handles completed name, not `.partial`). S3 staged `.s3-download-*.zip` has no journal; interrupted `downloadToStaging:296-338` leaves temp until manual cleanup. Journal exists only for **restore swap** (`532 Restore(truncate)` 3-phase `899 writeRestoreJournal` → `931 recoverInterruptedRestore`), not for backup staging.

---

### B-06 — `local.go:90 lockNamespace` — **STILL BROKEN — P1 split-brain (in-process only)**

**Ref:** phase-03 F-B-08, BKP-08-06, BKP-RV16-05

**LIVE proof:**
- `beacon/internal/backup/local.go:48-50` `namespaceMu map[string]*namespaceOperation{mu, refs}` + `90 lockNamespace(namespace)`: `91 namespaceMu.Lock()` → `92 operation := namespaceOps[namespace]` → `97 refs++` → `99 mu.Lock()` → returns `101 func(){ mu.Unlock(); refs-- }` — **per-namespace in-process `sync.Mutex` only**
- Guards **only** `Create:239` + `Restore:469`; **not** `List:415 / Get:442 / Delete:454 / Download:626` — concurrent `Delete` during `Create` races; cross-host: no persistent lock file, no `pg_advisory_xact_lock`, no S3 conditional `locks/backup-<server>.json` with TTL+refresh (mirror `restic/lock.go:47,124 refreshLocks`)
- `worker.go:182 pickupBackupJobs` limit 5 sequential `Execute` 20-min each inside `105` 1-min `tick` → 100-min blocks ticker; `Worker.tick:119` scans `ListAllEnabledPolicies` without advisory lock
- `store_backups.go:54` `UpsertBackup ON CONFLICT(server_id,name) DO UPDATE` overwrites concurrently created pending rows when two workers generate same `backup-%s:211 Format("20060102T150405Z")` second-truncated — second UUID overwrites first, then both daemons succeed on isolated `backupRoot`/S3 prefix with divergent `checksum` but same logical `name` race to `UpsertBackup`; S3 staged upload has no lock; restore journal path `local.go:895 ".<base>.restore-journal.json"` local-FS only → dual restore on shared `serverRoot` (NFS) creates independent journals, torn `parent/.rollback-*` orphans (`migratedInfo` etc.)
- `server.go:77` `backupMu sync.Mutex` serializes all creates on **one** beacon but not across hosts — insufficient for scaled daemon or host failover

**Effect:** Two beacons for same server (scaled daemon) can `Create` same `name` concurrently → isolated `backupRoot`/S3 prefix both succeed, two S3 objects same logical name different checksum race to DB. `ListBackups` `ORDER BY created_at DESC` hides duplicate-name collision until `Get` ambiguity.

---

### B-07 — `server.go:2155 backupProgressWS + server.go:1995 realtimeProxy only stats|logs|console` — **STILL BROKEN — UNWIRED dead end**

**Ref:** phase-03 F-B-15, BKP-08-08, BKP-RV16-13

**LIVE proof — beacon publishes but panel never proxies:**
- `beacon/internal/backup/backup.go:34` `BackupProgress{BytesProcessed,TotalBytes,Phase}` + `local.go:111` `reportProgress` (publishes to `ProgressFunc`); `Create:241` `creating 0,0` → `273 archiving 0,0` → `411 completed size,size` — **TotalBytes=0 for two of three phases**, final `info.Size()` only; no per-file increment, no hashed vs uploaded split, no `Throttle.ShouldOutput` guard (vs `kopia cli_progress.go:42 300ms` + `restic json.go:42 statusUpdate`)
- `beacon/server.go:62` `BackupProgressEvent="backup progress"` + `344 mux.HandleFunc GET /servers/{id}/ws/backup backupProgressWS` + `1542 SetProgressCallback→eventBus.Publish(BackupProgressEvent+":"+serverID)` + `2155 backupProgressWS` (`2169 ScopeWebsocket|ScopeBackupDownload`, `2192 Subscribe EventBus+":"+serverID`, `2203 writer.Write(msg)`, `25s pingWebSocket`) — **fully implemented**
- **Gap strictly in panel proxy:**
  - `forge/api/internal/http/server.go:1995` `v1.Get("/servers/:id/ws/stats", realtimeProxy(...,"stats"))` + `2006 ws/logs` + `2017 ws/console` — **no `ws/backup`** branch; `2408 IssueWSTicket` only for those three streams
  - `forge/api/internal/http/realtime.go:124` `realtimeProxy(cfg, ticketStore, stream)` → `167 inspectWSTicket(stream)` → `284-310 wsToken MintWebsocketToken` → `302 gorilla.DefaultDialer.DialContext(upstreamURL)` → requires caller `stream=="stats|logs|console"` — would fail ticket `Stream != stream` check if panel tried `backup`
  - `forge/web/lib/api/ws/websocket-manager.ts` supports factory functions but only for console/stats; `forge/web/components/server/backups-view.tsx:43` polls `refetchInterval 3000` when any `pending||running`, otherwise no polling, no `useWebSocket`, shows `pending` snapshot with `TotalBytes=0` until completion; `SetProgressCallback:78` overwrites single `progress` func pointer per adapter — concurrent backups for two servers on same beacon race

**Effect:** Integration 90% done but delivers no value — incident responders debug proxy instead of missing `TotalBytes` estimation. Even bridged today, WS would forward `0,0 archiving files` forever — adaptive ETA impossible; a 4-min backup shows zero movement then flips completed → users retry, creating duplicates that race `backupMu`. Kopia/Restic throttle 1/s + `timetrack.Estimator` for `%`/`Remaining` — Forge has none.

---

### B-08 — `scheduler/service.go:317 StorageLocality dead` — **STILL BROKEN — DEAD + vocab drift**

**Ref:** phase-05 F1 + BKP-08-09 + BKP-RV16-06, reverification-19 row 9

**LIVE proof:**
- `forge/api/internal/domain/domain.go:123` `PlacementRequest.StorageLocality string`
- `forge/api/internal/placement/strategy.go:42` `Candidate.StorageLocality` / `53 WorkloadRequest.StorageLocality` / `62 ScoreResult.StorageLocality`
- `forge/api/internal/services/scheduler/service.go:212` `FilterNodes:212 FilterNodes StorageLocality=="local_only" && RuntimeProvider!="local"` — hard filter (correct intent)
- `service.go:317-323` `ScoreNodes` `req.StorageLocality != "" && r.StorageLocality != req.StorageLocality → -1e10` penalty, `== → +1e8` bonus — scoring assumes locality can match
- `service.go:714-735` `nodeToCandidate:714 storageLocality="local"; if node.RuntimeProvider=="nfs"||"shared" → "shared"` else `"local"` — **candidates are `"local"/"shared"`**
- `evacuationplanner/service.go:28-33` enum `local_only / replicated / shared` (`StorageLocalOnly/Replicated/Shared`) — **evacuator vocabulary is `local_only/replicated/shared`**
- `forge/api/internal/http/handlers_servers.go:860` builds `PlaceServer` request field-by-field and **omits** `StorageLocality` — only debug `/placement/explain` JSON bindings carry it (`http/phase6_registrar.go:73`); `grep StorageLocality` shows zero production population except `domain.go:123` struct field + `scheduler/service.go:212/317` scoring
- `envaffinity/explain.go:104` builds candidates without `StorageLocality` — explain path understates locality effects

**Effect:** No production caller populates `PlacementRequest.StorageLocality` → `±1e10/1e8` scoring and hard filter are **dead code** (phase-05 F1 exact). If later wired, it **still cannot match**: candidates `"local"/"shared"` while evacuator enum is `local_only/replicated/shared`; request `local_only != local` → every node takes permanent `-1e10` penalty. Placement locality/mount-aware at schedule time per Longhorn (`diskSelector:4592/nodeSelector:4636` + `dataLocality disabled/best-effort/strict-local`) is instead checked **after** placement via `store_mounts_ext.go:297 ensureMountAvailableForServer` → “mount unavailable for this server node” post-hoc operator error while Longhorn prevents at schedule time.

---

## 2. Orchestration/Queue — 10 Signals (ALL STILL BROKEN)

### O-01 — `placement engine global mutex engine.go:38` — **STILL BROKEN — P1 throughput collapse**

**Ref:** final-parity subagent-09 row PLACEMENT/throughput, reverification-17 F9

**LIVE proof:**
- `forge/api/internal/placement/engine.go:18` `mu sync.Mutex` + `38-40 Place` `e.mu.Lock(); defer Unlock()` across `FilterByConstraints+Score+CheckSoft`, `79-81` `PlaceAll` same, `replica.go` reuses `e.scorer/e.checker` under caller locks
- `scheduler/service.go:282-294` fetches `NodeCapacitySnapshot` **sequentially** per node — per-node queries sequential; `strategy.go:107-109` unstable `sort.Slice` tie-break (no `ShuffleNodes`)
- `engine.go:18` `ConstraintChecker struct{}` (`constraints.go:32`) and `LeastLoaded/BinPack/SpreadScorer` are **stateless**, `RandomScorer` owns its own `mu:149-157` — mutex protects nothing contended; likely removable outright
- Reference throughput: Nomad lazy iterator chains + `stack.go:79-100` `ShuffleNodes + limit=log2(n)` cap (2 for batch) power-of-two + `ShuffleNodes:82` + concurrent workers with optimistic `Plan` commits (`generic_sched.go:291,309`) — Forge has none; single mutex serializes whole cluster

**Effect:** N concurrent deploys serialize 1-at-a-time; tail latency linear in fleet size. No `git diff HEAD -- placement/**` semantic fix.

---

### O-02 — `constraints.go:59 +1e12` — **STILL BROKEN — P0 score normalization destroyed**

**Ref:** final-parity subagent-09 row 1, reverification-17 F1

**LIVE proof:**
- `placement/constraints.go:59-63` satisfied `+= 1e12`, unsatisfied `-= 1e10` — soft bonus dwarfs everything
- `strategy.go:91-104` `LeastLoadedScorer` base `≤3` (sum of 3 ratios), `SpreadScorer:141` `≤1`, `RandomScorer:162-173` `≤1`
- `scheduler/service.go:306-323` injects `+1e9` preferred, `-1e10` storage-locality mismatch, `+1e8` match, and `predictive.go:149-175` multiplies/adds `trend/affinity` atop already-scaled `r.Score`
- `ExplainScores:89-106` reports `SoftConstraintBonus: 1e12` next to `BaseScore: 1.7` — incommensurable

**Math:** One soft satisfier (+1e12) dwarfs one unsatisfied (-1e10) by 990 B; two unsatisfied (-2e10) still loses to one +1e12. Storage mismatch penalty (-1e10) exactly cancels one soft penalty and sits between soft scales — tuned independently, not normalized. `ExplainScores` mixes units.

**Reference:** Nomad `rank.go:1004-1038` averages every factor into `[-1,~18]` then into `[-1,1]` per factor; per-factor breakdown into `AllocMetric.ScoreMetaData` (`generic_sched.go:582`) — Forge has unbounded additive bonuses where constraint count dominates binpack/fit; anti-affinity `replica.go:170-175` `0.1*count` never survives.

---

### O-03 — `replicamanager retry every 60s` — **STILL BROKEN — P0 crash-loop amplify / reschedule storm**

**Ref:** final-parity subagent-09 row 3, reverification-17 F6

**LIVE proof:**
- `forge/api/internal/services/replicamanager/service.go:705-719` `Start:706 time.NewTicker(1*time.Minute)` drives `reconcile`; `732-783` `if hasFailed>0 RetryFailedPlacements`; `779-783` conditional
- `RetryFailedPlacements:883-910` iterates **every** app, every `failed` instance, `ReplaceInstance:561-652` marks `removing→place→dispatch→provisioning` with **no** `ReschedulePolicy` analogue; `904 time.Sleep(100*time.Millisecond)` per app **inside** the shared reconcile goroutine, stalling status reconciliation for all other apps
- `ReplaceFailedInstance:599-688` builds candidates via `FilterNodes` with empty region/runtime, never excludes/penalizes `inst.NodeID` — flaky-but-online node retains high free capacity after workload died → re-selected immediately (Nomad feeds prior failures into `PenaltyNodeIDs:797-818`)
- No `structs.go:6611` `ReschedulePolicy{Attempts,Interval,Delay,DelayFunction(fibonacci/exponential),MaxDelay,Unlimited}` + `generic_sched.go:272-285` `WaitUntil` delayed follow-up evals + `reconcile_cluster.go:1408` `createRescheduleLaterEvals` + `rank.go:875-910` `NodeReschedulingPenaltyIterator`

**Effect:** Bad image fleet-wide → permanent 1/min `ReplaceFailedInstance` per failed instance, each dispatching `dispatchBeaconCommand:834-845` and appending `placement_decisions` — churning Beacon commands + decisions. Even `1<<retry_count` can overflow PG integer for large retry_count (`store_backup_jobs.go:199`). Inline sleep amplifies stall.

---

### O-04 — `queue/store.go:113 Cancel no predicate` — **STILL BROKEN — P0 audit-trail corruption / silent cancel evaporation**

**Ref:** final-parity subagent-09 row 11 Q-STILL-01/LF-1, reverification-18 LF-1

**LIVE proof:**
- `forge/api/internal/services/queue/queue.go:247-254` `Cancel(ctx,id) { s.activeMu.Lock(); cancel(); s.store.Fail(ctx,id, errors.New("job cancelled")) }` — always calls `Fail`, no status gate
- `queue/store.go:113-130` `Fail(ctx,id, jobErr)` → `UPDATE job_queue SET status='failed',error=$2,completed_at=$3,locked_by=NULL,locked_until=NULL WHERE id=$1` — **no** `AND status IN ('pending','running','retrying')` guard
- `queue/store.go:98-111` `Acknowledge` likewise unguided `WHERE id=$1` — cross-instance race: A cancels → `failed`; B succeeds → `Acknowledge` overwrites cancel to `completed` (both directions lose)
- `operation/store.go:210-214` *does* guard `WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')` but `operation/service.go:307` `UpdateStatus(StatusSucceeded)` still overwrites concurrent `Cancel` because completion is not CAS against `running→cancelled` — `Service.Cancel:310` cancels context then calls store, but completion path has no `CAS`

**Reference:** River `JobCancel` `FOR UPDATE` refuses final states, running → `cancel_attempted_at` metadata honored by `JobSetStateIfRunningMany:619-698` (`rivertype`); Nomad revocation via modify-index — Forge has no equivalent. `MASTER_FINDING_INDEX REF-ORCH-Q-01` preserved.

---

### O-05 — `operation fake backoff` — **STILL BROKEN — P1 slot starvation + thundering herd**

**Ref:** final-parity subagent-09 row 12 Q-STILL-02, reverification-18 LF-2

**LIVE proof:**
- `forge/api/internal/services/operation/service.go:283-306` on failure: `288 backoff := BaseBackoff; for i:=1; i<attemptCount; i++ { backoff*=2 ... }` → `296 timer := time.NewTimer(backoff)` → `select { case <-ctx.Done(): UpdateStatus(Cancelled); case <-timer.C: UpdateStatus(Retrying)}` — **blocks one of 5 workers up to 30s** (`service.go:106 MaxBackoff`)
- `operation/store.go:36-48` `Dequeue:36 WHERE status IN ('queued','retrying') ORDER BY CASE WHEN status='retrying' THEN 0 ELSE 1 END` — **instant priority for `retrying`** → actual delay is only the blocked goroutine's sleep; once marked `retrying` any worker's next 1s poll picks it (`service.go:211`) with **no `available_at`/`next_retry_at` column** (`092_durable_operations.sql:23-36` has none), unlike `job_queue` which already solves via `retrySQL:66 available_at=$3` (`queue/store.go:64-67`)
- No jitter → thundering herd on mass failure; River uses `attempt^4 ± jitter` (`retry_policy.go:49-103`)

**Forge irony:** `queue.retrySQL` solves it correctly; `operation` siblings need `next_retry_at TIMESTAMPTZ` + `WHERE next_retry_at<=NOW()` and never sleep in worker.

---

### O-06 — `queue/periodic.go RFC3339Nano divergence` — **STILL BROKEN — P0 N× periodic spam**

**Ref:** final-parity subagent-09 row 10 Q-STILL-03/LF-3, reverification-18 LF-3

**LIVE proof:**
- `queue/periodic.go:50-60` `PeriodicInterval(d) t.Add(d)`; `62-81` `Add:78 job.nextRun = schedule.Next(now)` per replica start time — seed is each replica's `now`
- `periodic.go:109-126` `tick` holds `nextRun` in process memory, fires `go execute()`; `128-147` `idempotencyKey := fmt.Sprintf("periodic:%s:%s", job.id, scheduledFor.UTC().Format(time.RFC3339Nano))` — **key derives from each replica's own `nextRun`**
- No elector: `queue/leader.go:23-91` `Elector` exists only in dead fork `forge/api/queue` (see O-08), grep `leader|Elector` in `forge/api/internal` returns only jitter comments (`reconciler/service.go:187-189`, `heartbeatmonitor/service.go:140-142`, `observability/service.go:38-40` *"Jitter 0-5s to avoid thundering herd"*) — jitter desynchronizes writes, does not deduplicate
- Two replicas started 30s apart produce **different keys for same logical hour** → `ON CONFLICT DO NOTHING` fails to dedupe → both enqueue
- River contrast: `river_leader` TTL row + `LeaderAttemptElect:577` gates periodic enqueuer on leader only; Nomad `leader.go:413-430` leader-only `periodicDispatcher`

**Effect:** At `main.go:542-546` two periods (`backup.retention` hourly `backup.retention` + `cert.renewal` daily) fire N times with N replicas → N× job spam on 2nd instance, silent until scale-out.

---

### O-07 — `eventstore/outbox.go:39 zero subs` — **STILL BROKEN — P0 write-only durability (AF-1)**

**Ref:** final-parity subagent-09 row 14 AF-1, reverification-18 AF-1

**LIVE proof:**
- No tx-bound publish API: `EventStore.Publish:113-133` signature `(ctx, Envelope)` with `s.db.Exec` on pool; `OutboxPublisher.Publish:214-222` does `store.Publish` then `registry.Publish` `events/registry.go:144-215` fan-out with `wg.Wait()` 30s — **no `PublishTx(ctx,tx,env)`**; grep `PublishTx|WithTx` across `forge/api` returns **zero**
- Business transactions commit first, then event inserts after (`git/deploy_service.go`, `dbbackup/service.go:262-284`, `recovery/service.go:723` `c.publisher.Publish` after business row) — if process dies between commits, durable copy absent
- Relay has zero production subscribers: `Relay.Subscribe:39-43` prod grep: **only** `internal/eventstore/store_test.go:362,395` plus all real consumers on `events.Registry` (`cmd/api/main.go:441 fenceSvc`, `946 failSvc`, `1026-1077 tm/lb/ingress/obs/whSvc`, `1236-1243 enhancedNotif`); `http/server.go:148` `EventRelay *eventstore.Relay` wired at `main.go:1519` but zero dereferences elsewhere
- `Relay.processEvent:151-158` snapshots `subs` empty → `deliverWithRetries:174-207` iterates zero handlers → `lastErr==nil` → returns nil → `processBatch:160-171` **marks `dispatched` within 5s poll** (`102-105 ClaimPending lease 330s` but mark eager) — every event claimed then marked dispatched having been delivered to **nobody**
- Real consumers are in-memory only: `registry.go:144-215` fan-out blocks publisher, retries 3× doubling jitter, dead-letters to 10k map — vanishes on restart; cross-instance automation impossible (`NodeOffline` on A cannot trigger failover on B); `events` table is 7-day audit log (`outbox.go:95 Prune 7d/30d`) at price of two writes per publish with no replay

**Reference:** River `InsertTx:1831-1854` binds enqueue to business tx; Nomad Raft → broker ephemeral above it — Forge has durable primitive with correct lease math `(batch+1)*30s=330s` but lacks both ends.

---

### O-08 — `queue/leader.go:23 unwired` — **STILL BROKEN — P0 ~30 uncoordinated daemons (AF-2)**

**Ref:** final-parity subagent-09 row 15 AF-2, reverification-18 AF-2

**LIVE proof:**
- Live coordination in `forge/api/cmd/api/main.go:521-1304` per `Start(appCtx)` grep — `queueSvc.Start:533` + `periodicScheduler.Start:1194` + `opSvc.Start:787` + `gitOpsController:521` + `healthCheckRunner:859` + `discoverySvc:990` + `ingressSync:995` + `cleanupSvc:1016` + `tmSvc:1028` + `lbSvc:1072` + `buildSvc:1106` + `buildpackSvc:1109` + `cronJobSvc:1147` + `resMgr:1150` + `hbm:1151` + `rec:1156` + `mig:1157` + `ep:1158` + `mailWorker:1159` + `whSvc:1160` + `failSvc:1161` + `bkWorker:1164` + `eventRelay:1167` + `periodicScheduler:1194` + `procedureSvc:1196` + `replicaMgr:1197` + `autoSvc:1198` + `enhancedNotifSvc:1235` + `pipelineSvc:1304` + inline session-cleanup `:1203` + domain startup sync `:1030` ≈ **28-30 background loops per API replica**, none leader-gated
- Only observed coordination: jitter comments `hbm.Start:140-142` / `reconciler/service.go:187-189` `rand.Intn(5000)` *"desynchronize heartbeat evaluation"* — desynchronizes, does not deduplicate
- Dead elector: `forge/api/queue/leader.go:23-133` `Elector{config ElectInterval 5s TTL 15s, LeaderAttemptElect/Resign, isLeader atomic, leaderCh}` fully correct but **header at `queue/doc.go:1-5` + `cmd/api/river.go:13-20` stub: "River is not the canonical durable queue… not wired in main.go"** — zero references from `cmd/api/main.go`; `migrations/137_river_queue_features.sql:25-35` `river_queue` table zero writes from live code (only inside fork `queuedriver/pgx:498`); `river_leader` TTL rows (`elector.go:26-31`, `dbsqlc/river_leader.sql:1-60`) likewise unwired; only advisory-lock proof-of-pattern is `store_failover.go:130-165` `pg_advisory_xact_lock(hashtextextended(nodeID))` for incident dedupe (applied once)

**Effect:** Duplicate `backup.retention`/`cert.renewal` (O-06), N× reconcile/evacuation, racing restart loops (`failover lastAction map in-memory:99,446` per-process), thundering herd on 2-instance restart.

---

### O-09 — `fencing wrong edge` — **STILL BROKEN — P0 inverted fence + unenforced (AF-3)**

**Ref:** final-parity subagent-09 row 16 AF-3, reverification-19 row 14

**LIVE proof:**
- **Wrong edge:** `fencing/fencing.go:20-27` `switch envelope.Type { case EventNodeRecovered: return FenceNode }` vs `main.go:441` `eventRegistry.Subscribe(EventNodeRecovered, fenceSvc)` — fires on **recovery**, not failure; correct STONITH/fencing fences *before* replacement on `EventNodeOffline/Unreachable` (`heartbeatmonitor/service.go:266-312 classify:335 EventNodeOffline` when `misses >= OfflineThreshold`, `340 EventNodeRecovered` on healthy recovery). NetBird revokes on policy removal via next `NetworkMap` push (`account.go:457`) — enforcement immediate, not post-recovery; Incus `heartbeat.go:113` `offlineThreshold` drives evacuation
- **Unenforced:** `fencing.go:29-48` `FenceNode` loops `ListServersForNode → Generation++ + 24h leaseExpiry → UpdateServerGeneration` (duplicated by `recovery/service.go:642-650` second bump `+1h` lease + `653-657` rollback `UpdateServerGeneration(prevGen, prevLease)` and `670-677 errors.Join`); read side near-zero: `reconciler/service.go:559-565` stale-workload counter only; `beacon/internal` grep `generation|fence` yields **zero** except logrotate; power ops `clustermanager/service.go:671-678` builds `CreateServerRequest{Target:{NodeID,…}}` without passing `generation`; `beacon/server.go:735-842` create/power handlers never read generation token — beacon trusts any generation
- **Racing & duplicated:** Two writers race; last writer wins; leases differ 24h vs 1h; no CAS: `store_servers.go:502-506` `UPDATE servers SET generation=$2 … WHERE id=$1` unconditional — no `WHERE generation=$n`; six controllers can act on same unhealthy server within one interval: reconciler desired-vs-actual (`523-535`) + health-recovery restarts (`581-676`) + replicamanager replacement (O-03) + crashdetector auto-restart (`main.go:947-970`) + failover evacuate/restart (`862-924` go func 2-hour context without leader) + recovery restore (`335-372`)

**Effect:** Partition window is unfenced (stale node keeps `generation` unchanged, continues accepting commands); recovered node gets fenced *after* reconnect → legitimate node rejected. Split-brain lets stale node accept writes; even later wiring would require `Fence{Server,Node}(CAS WHERE generation=$old)`, `clustermanager` token check before dispatch, beacon rejection of stale generation — none present.

---

### O-10 — `no TunnelIP` — **STILL BROKEN — MISSING overlay mesh (P1)**

**Ref:** final-parity subagent-09 row 18, reverification-19 rows 7-8

**LIVE proof — no tunnel/mesh IP modeled anywhere:**
- `forge/api/internal/store/store.go:132-213` `type Node struct` lists `AllowedIPs []string`, `NetworkInterface string`, `RuntimeProvider string` but **no** `TunnelIP`, `WGIP`, `NetBirdIP`, `VNI`, `OverlayCIDR`; `store_nodes.go:71-81 SELECT` likewise no tunnel column → `grep TunnelIP|tunnel_ip|tunnelIP` across `forge` returns **zero** for live store (only `crossnode_test.go:843-857` literal `"WireGuard"`)
- `services/crossnode/resolver.go:98-110` `GetNodeHost` tries `publicHostname` then `fqdn`, returns `"" → 110 return "localhost"`; `resolveFromStore:73-82` caches even insecure fallback for 30s
- `services/crossnode/resolver.go:135-165` `resolveFromDiscovery` prefers discovery but falls back to *any* endpoint (not just healthy) then to public host — never tries tunnel/overlay; `trafficmanager/service.go:665-671` `resolveTargetHost` picks public only; `crossnode/resolver.go:98-106` fallback to public → `localhost` when disconnected
- `services/servicediscovery/registry.go:64-120` `RegisterEndpoint` writes + persists, but `stale_reaper.go:27` `heartbeatTTL:3m interval:30s reap:100-117` only does `UpdateEndpointStatus(..., Unhealthy)` — never `RemoveEndpoint`; beacon never registers (`grep servicediscovery under beacon/*` zero); `verify` dials from panel not source

**Effect:** Gateway & migration (`migration/service.go:450-467 NodeURL`) & admin console ride **public** internet even inside private DC; overlay/MAGIC-WAN/NetBird mesh unavailable even if operator deploys NetBird externally (Forge has no place to store `tunnel_ip,mesh_pubkey` to distribute via heartbeat; `resolver.ResolveTargetHost` cannot prefer it). Private east-west promise vs reality is marketing-level.

---

## 3. Cross-Cutting Gaps Still Broken (supporting signals)

These were not in the 18-item task list but are load-bearing for the same subsystems and remain **STILL BROKEN** — cited for completeness (no double-count):

| Concern | LIVE line | STATUS |
|---------|-----------|--------|
| Transaction boundaries / outbox torn | `queue/store.go:69-149` `Dequeue/Ack/Fail/Retry` 4 independent `Exec`s on pool (no `Begin`), only `Enqueue:21-47` is tx; `dbbackup/service.go:257-284` restore dispatch after business commit | **BROKEN** — state tear on crash mid-sequence (reverification-18 LF-4) |
| Worker lease steal semantics | `queue/queue.go:213-226 keepLease` uses `jobCtx` for `Heartbeat` — `Stop:132-142` cancels → heartbeats stop → 30s lapse → another replica steals draining jobs; sibling `operation/service.go:500-504` already fixed via `context.WithoutCancel` — port not applied | **BROKEN** LF-8 |
| Idempotency namespace fragmentation | `queue/queue.go:239 "forge-job:"+key` vs `operation/service.go:342 "forge-op:"+kind+":"+key` vs `368 "forge-op:"+key` (no kind) + step `"step-"+opID` vs `"forge-operation-step:"+jobID`; `ON CONFLICT DO NOTHING` silent — same key creates 2 jobs across engines | **BROKEN** LF-5 |
| StorageLocality dead (already in B-08) + mount after placement | `scheduler/service.go:126-150` reservation only CPU/Mem/Disk; `store_mounts_*` not consulted; `nodeToCandidate:703` `AllocatedDisk` = sum `servers.disk_mb` not `df`; `evacuationplanner.StorageLocality:691-693` `if mountStore==nil return Replicated` default (local → replicated misclassify → `AutoReplace` data-loss) | **BROKEN** reverification-19 F2/F3/F5 |
| Runtime honesty phantom providers | `forge/api/internal/runtime/runtime.go:9-16` 7 names vs `beacon/internal/runtime/factory.go:19-31` 5 (no lxc/kvm); `beacon/server.go:735-842` no `Provider` field → always `mode:docker:841`; `capabilities.go:118 FirecrackerCapabilities{ Snapshots:true}` but no snapshot op; `multiruntime.go:45-52` silent fallback to default | **FALSE** R-01..03 (reverification-19 Findings 1-2) |
| Service discovery write-only | `servicediscovery/registry.go:91 LastHeartbeat` set once at admin `handlers_servicediscovery.go:55-70` (admin only); `stale_reaper.go:104-107` 3m TTL marks **all** unhealthy; beacon never registers | **UNWIRED** R-05 |

---

## 4. Severity Roll-Up (still broken)

**P0 (fix before 2nd replica or real evacuate):**
- B-01 plaintext beacon + null AAD transplant
- B-07 dead backup progress proxy (panel never proxies — 90% built, unwired)
- O-02 score overflow `±1e12` (placement unusable at scale)
- O-03 reschedule storm (60s forever, no cap/backoff)
- O-04 cancel CAS (terminal regression)
- O-06 periodic N× spam (silent until 2nd instance)
- O-07 write-only durability (AF-1) + O-08 no leader (AF-2) + O-09 fencing inverted/unenforced (AF-3)

**P1:**
- B-02 OOM streaming
- B-03 retention AND vs OR (data-loss)
- B-04 SQL-only prune / B-05 orphan .partial / B-06 split-brain lock
- O-05 fake backoff + O-10 no TunnelIP / no overlay

**P2:**
- B-08 StorageLocality dead vocab drift (dead code implying enforcement, actually post-hoc `ensureMountAvailable`)

---

## 5. What *Is* Correct (keep verbatim — do not rewrite)

- Beacon 3-phase restore journal `local.go:532-623` (`prepared→live-moved→activated` with `writeRestoreJournal:899` + `recoverInterruptedRestore:931` + `RecoverRestoreJournals:867` + `syncDirectory` after each rename) — arguably **stronger than Kopia/Restic** for crash consistency of destructive restores
- `store_backups.go:138-162` `DeleteBackup FOR UPDATE` + `RenameBackup FOR UPDATE` + `is_locked` guard — correct Tx for delete/rename (but not used by the SQL-only prune paths)
- `queue/store.go:49-67` steal≠retry contract (`must NOT mutate retry_count`) + `retrySQL` sole `available_at` writer — **better than River** (regression-tested `retry_accounting_test.go:8-27`); keep contract, port to `operation` path
- Hysteresis classifiers, generation concept, `SKIP LOCKED` claims already correct primitives — need wiring, not replacement

---

## 6. Conclusion

**Phase-03 synthesis 18 findings + final-parity subagent-08 20 rows + subagent-09 19 rows + reverification-16/17/18/19 are all reconciled and re-anchored — none are double-counted, none are fixed.** The 18 task-mandated signals (8 backup: `encryption.go:92,150`, `retention.go:61`, `store_backups.go:240`, `local.go:90,275`, `server.go:2155/1995`, `scheduler/service.go:317` + 10 orchestration: `engine.go:38`, `constraints.go:59`, `replicamanager:60s`, `queue/store.go:113`, `operation fake backoff`, `queue/periodic.go RFC3339Nano`, `eventstore/outbox.go:39`, `queue/leader.go:23`, fencing wrong edge, no `TunnelIP`) are **STILL BROKEN** at HEAD. All required remediation (per-backup `HKDF(activeKey, serverID:uuid, salt=random16)` + `forge:v1:<keyID>:nonce||Seal(AAD=serverID:backupName)` + chunked AEAD streaming + union-OR retention + `pg_advisory_xact_lock` prune + `.partial` reaper + `PublishTx` + Relay subscribers + leader election gating maintenance + `queue.Service` sole writer + CAS fencing on `generation` + `TunnelIP/mesh_pubkey` columns) remains **unapplied**. Agree with reverification-16 aggregate: ≥13 broken/dead rows out of 15 parity rows — state persists.

**Activation order if fixing now (no new subsystem, existing primitives):**
1. Wire `backupProgressWS` end-to-end (`backup` to `realtimeProxy` union + `IssueWSTicket` + `lib/api.ts` union) + throttle `reportProgress` 1/s + set `TotalBytes` via early `Stat`
2. Replace `io.ReadAll` with chunked AEAD streaming via `UploadStream/DownloadStream` + per-backup HKDF/AAD envelope via `keyring`
3. Unify retention to single `RetentionEngine` union OR (`retrySQL` already proves pattern) + `SELECT FOR UPDATE SKIP LOCKED` + `DELETE RETURNING` + storage delete in same Tx
4. Gate every prune with `pg_advisory_xact_lock('backup_prune:'||serverID)` + nightly `ListObjectsV2 vs SELECT` reconciliation + `.partial/.restore-*/.s3-download-*` reaper
5. Add `PublishTx(tx,envelope)` + subscribe `fencing,failover,tm,lb` to **Relay** (not just `Registry`) + stop marking dispatched when zero handlers
6. Resurrect `queue/leader.go` elector or `pg_try_advisory_lock('forge-leader')` gating maintenance daemons (keep per-event `SKIP LOCKED` un-gated)
7. Single `fencing.Fence{Server,Node}(CAS WHERE generation=$n)` + `clustermanager` token check + beacon stale-generation rejection; unify `StorageLocality` enum once (`local_only/replicated/shared`) + populate from mount analysis or delete dead scoring; add `nodes.tunnel_ip,mesh_pubkey` + `resolver.ResolveTargetHost` prefers tunnel IP; consolidate to `queue.Service` sole writer (decommission `operation` dequeuer, keep read-model)

*Generated by Phase 02 Agent 09/10 — parallel lane 09/10. No files under `forge/` or `beacon/` were modified. All 18 signals re-verified BROKEN with exact file:line citations above.*
