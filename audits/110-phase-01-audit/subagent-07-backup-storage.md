# Subagent 07 — Backup & Storage — Phase 01 Preparation Audit

**Agent:** 07/10 — Backup & Storage (encryption, integrity, retention, pruning, restore, progress, locality)
**Date:** 2026-08-24
**Scope:** `forge/api/internal/services/backup/*` , `beacon/internal/backup/*` , `beacon/internal/server/server.go:2155` `backupProgressWS`, `forge/api/internal/store/store_backup*.go`, migrations `*backup*`, `forge/web/components/server/backups-view.tsx:43`, `forge/api/internal/http/handlers_backup*` + `handlers_servers.go` backup handlers, `forge/api/internal/http/realtime.go` proxy.
**Evidence base:** `audits/FINAL_PARITY_AUDIT.md` §9 BK-01..BK-16 (pp.230-252), `audits/phase-03/synthesis.md` + `phase-03/subagent-02-backup-crypto-retention.md`, current checkout file:line re-verification (no code modified).

> **One-line verdict:** Beacon's on-disk journal/restore is the strongest parity in the repo; Forge's backup-as-a-service layer above it is the weakest — three retention engines with opposite AND/OR semantics, SQL-only prune that orphans remote objects, nil-salt/nil-AAD GCM that enables cross-server transplant, whole-archive `io.ReadAll` encryption that OOMs, and a publish-but-never-proxy progress WS that makes backups appear dead. All P0s are activation/wiring, not green-field builds.

---

## 1. Inventory — what exists, where, how it is wired

### 1.1 Forge — API service layer (`forge/api/internal/services/backup/`)

| File | LOC | Role | Status vs. reference |
|------|-----|------|----------------------|
| `encryption.go:1-239` | 239 | Forge-side single-archive AES-256-GCM + HKDF + `EncryptReader`/`DecryptReader` pipe adapters, `deriveEncryptionKey` HKDF, `newGCM` | **BROKEN** — `deriveEncryptionKey:92-100` `hkdf.Key(..., nil, purpose, 32)` nil salt; `Encrypt:119` `gcm.Seal(nil, nonce, data, nil)` nil AAD; `EncryptReader:150-176` / `DecryptReader:178-204` `io.ReadAll` full buffer (OOM); nonce persisted hex `service.go:311-313`; decoupled from `secrets/keyring.go:75` AEAD envelope |
| `service.go:1-902` | 902 | Legacy monolith `Service` — `UploadBackupWithOptions:271-351`, `DownloadBackupWithOptions:357-395`, `CleanupExpiredBackups:689-741`, `EnforceRetentionPolicy:743-784`, `VerifyChecksum:438-489`, `RestoreFromStorage:564-625`, storage adapter registry `adapters:172` | **DUPLICATE + BROKEN** — coexists with `MainService` quartet; `UploadBackupWithOptions:303` `io.ReadAll` + whole-object `adapter.Upload`; `EnforceRetentionPolicy:769` `withinCount && withinAge` AND vs Kopia/Restic OR; `CleanupExpiredBackups:706` iterates `ListAllEnabledPolicies` + `ListExpiredBackups` (hard-coded 30d) |
| `main_service.go:1-1160` | 1160 | `MainService` quartet `ConfigService+JobService+ArtifactService+RestoreService`, provider registry, retention `RetentionPolicy` type `CreateRetentionPolicy:543`, scheduling `ScheduleBackup:292`, storage `LoadStorageProviders:454` | **DUPLICATE side of service.go:271** — stated winner (phase-03 synthesis §6, audit §9). Routes via `ArtifactService`/`RestoreService` but `Worker` still drives legacy `Service` |
| `storage.go:1-380` | 380 | `StorageBackend` iface + `LocalStorageBackend` hardened `securePath:244-273` (`EvalSymlinks+Rel` escape check, `0700`/`0600`), `StorageManager`, path helpers `CreateStoragePath:336` | **COMPLETE+ (hardened path)** |
| `storage_s3.go:1-938` | 938 | Full GCS/Azure/S3/Local matrix — `S3StorageAdapter:40`, `AzureStorageAdapter:389`, `GCSStorageAdapter:549`, `LocalStorageAdapter:795`, `adapter split comment:27-37` | **PARTIAL** — S3 path correct, GCS JWT `567-574` 2m timeout+redirect guard correct, but beacon only has Local+S3 (deliberate split, correctly documented). `LocalStorageAdapter:802` `MkdirAll 0755` vs `StorageBackend` `0700` incoherent; Forge `S3StorageAdapter:48` no endpoint validation vs beacon `s3.go:100` strict |
| `compression.go:1-257` | 257 | `CompressReader`/`DecompressReader` pipe wrappers for gzip/zstd, env `BACKUP_COMPRESSION_ALGORITHM` | **PARTIAL** — `CompressReader:63` pipe correct but `service.go:285` composes `CompressReader` → `EncryptReader` then `io.ReadAll` defeats streaming; test `encryption_test.go:146` certifies wrong order (encrypt→compress) |
| `interfaces.go:1-146` | 146 | `StorageAdapter:86` iface (Upload/Download/Exists/List/UploadStream/DownloadStream/GetFileInfo), `SlogLogger` | **COMPLETE** — contract exists, `UploadStream`/`DownloadStream` unused on encrypted path |
| `adapters.go:1-74` | 74 | Provider factories `NewS3Factory:7`, `NewGCSFactory:33`, `NewAzureFactory:50`, `NewLocalFactory:68`, env `forge:v1` keyring mapping | **COMPLETE** — small registry pattern phase-03 recommends adopting |
| `artifact.go:1-1061` | 1061 | `ArtifactService` — `Create:143`, `CleanupExpired:686`, `ApplyRetentionPolicy:710`, `Verify:516`, `Download:591`, `deleteFromStorage:788`, `downloadFromBeacon:813`, beacon-remote `resolveBeaconArtifact:824` | **PARTIAL/BROKEN** — `Verify:522` early-exit `if artifact.IsVerified {return nil}` never re-verifies; `downloadToTempFile:877` pulls whole object to `/tmp`; `ApplyRetentionPolicy:731-744` `exceedsCount \|\| exceedsAge` correct per-artifact but paginated `List Page:1 PerPage:200` then slice diverges from DB ordering; `pre-restore snapshot:733` creates logical record not actual file |
| `restore.go:1-1292` | 1292 | `RestoreService` — `Create:186`, `Execute:354`, `completeDirectRestore:503`, `execute*Restore:779-1152` via daemon, `verify*Restore:1154`, `Rollback:684`, `createPreRestoreSnapshot:733` | **PARTIAL** — beacon-direct path `381-443` (`RestoreBackup`/`RestoreDatabase`) correct; fallback `beaconClient.ExecuteRestore` correct but `determineNodeForDatabase:1229` raw `db.QueryRow` scan brittle; `RollBack` re-enters `Execute` sync accumulating temp dirs |
| `worker.go:1-458` | 458 | `Worker` — `Start:57`, `tick:110` 1m ticker, `policyDue:152-162`, `persistNextRun:166-176`, `pickupBackupJobs:182-199`, `enforceRetentionBeforeBackup:401-448` | **PARTIAL** — `policyDue:156-162` `NextRunAt==nil→false` correctly avoids instant-fire but first tick silently initializes (requires external bootstrap); `tick:119-140` runs `ListAllEnabledPolicies` each minute sequentially; `pickupBackupJobs:186` `ListRetryableBackupJobs(5)` with 20m per-job timeout serial — no worker pool but single goroutine acceptable at panel scale; `enforceRetentionBeforeBackup:422` `overLimit=count-MaxBackups+1` correct but second-pass `ListBackups(1,1000)` without `FOR UPDATE` races |
| `config.go` , `job.go` , `retry.go` , `job_test.go` etc. | — | `BackupConfig` template + `BackupJob` lifecycle + retry `withRetry` 3×1s→30s | Validated; config defaults `MaxBackups 10/RetentionDays 30` `config.go:170-175` with `Validate:492` vs beacon `RetentionPolicy` 0=disabled diverge (see §4) |

**There is no `forge/api/internal/services/backup/retention.go`.** The task prompt's `retention.go:61 AND` refers to `beacon/internal/backup/retention.go:61`. Forge retention lives inline in `service.go:743` and `artifact.go:710`. Duplication is itself a finding.

### 1.2 Beacon — daemon backup (`beacon/internal/backup/`)

| File | LOC | Role | Status |
|------|-----|------|--------|
| `backup.go:1-163` | 163 | `AdapterType` `LocalAdapter/S3Adapter`, `BackupInfo`, `BackupInterface:60-69`, `BackupManager`, `validNamespace/validBackupName`, `canonicalDirectory:138` | **COMPLETE** — `validBackupName:125` requires `.zip`, length ≤128, `filepath.Base==name` no slashes/backslashes correct |
| `local.go:1-1026` | 1026 | `LocalBackup:41` under `backupRoot/<namespace>`, `lockNamespace:90`, `reportProgress:111`, `namespaceDir:120`, `migrateLegacyBackups:138`, `Create:238-413` zip via `zip.NewWriter(zipTarget)` + `rateLimitedWriter`, `Restore:468-624` 3-phase journal, `Download:626`, `validateArchive:645`, `extractArchive:685`, `selectRestoreEntries:731`, `readOrCreateMetadata:757`, `RecoverRestoreJournals:867`, `writeRestoreJournal:899`, `recoverInterruptedRestore:931` | **COMPLETE+ for storage+restore; BROKEN for integrity+progress** — `Create:275-395` correct staging `O_CREATE|O_EXCL → Rename` + `Sync` + `calculateChecksum` + `writeMetadata`; `Restore:532-624` 3-phase `prepared→live-moved→activated` + `syncDirectory` strong; BUT `local.go:275` `.partial` never indexed/GC'd, `reportProgress:111` per-chunk unthrottled, `readOrCreateMetadata:757-785` trusts sidecar without mandatory rehash |
| `retention.go:1-134` | 134 | `RetentionPolicy{MaxBackups,MaxAge,KeepDaily/Weekly/Monthly}`, `Apply:35` with sorting, `MaxAge`/`KeepX` loops, `MaxBackups` intersect, `keep[mostRecent]=true:122` safety rail | **BROKEN (P1 inversion)** — `61-62` comment explicitly `Intersection: must satisfy ALL rules` → AND vs Kopia/Restic OR; loops `79-102` re-set `keep=true` for buckets even when `MaxAge` excluded, then `104-118` intersect with `MaxBackups` — contradictory; `Apply:27` rejects negative as error correct, `52-54` `CompletedAt.After` sort correct |
| `s3.go:1-414` | 414 | `S3Config:30`, `S3Backup:52` with `local *LocalBackup` staging, `NewS3Backup:69-98` fail-closed + `validateS3Endpoint:100-117` (HTTPS-only except loopback), `Create:126-161` local stage→`uploadToS3` 3-attempt `retryBase:1s`, `List:163-207` Head per object, `downloadToStaging:288-375` `LimitReader(maxS3DownloadBytes 50GiB:27)` + `ensureStagingDiskSpace`, `getS3Key/getS3Prefix`, `checksumFromMetadata:396` | **PARTIAL / BROKEN** — `NewS3Backup:69-98` correctly fails closed (nil/empty checks, `LoadDefaultConfig` + `RetryMaxAttempts 5`), `validateS3Endpoint:100` correct; BUT `Create:135` `defer s.local.Delete` deletes staging even on 3-attempt failure (good) yet no encryption on beacon path (plaintext zip staged, metadata `sha256:27` user-metadata only); `List:187-190` Head per object N+1 without pagination limit beyond paginator |
| `verification.go:1-118` | 118 | `VerificationResult`, `VerifyBackup:31` hash-compare `info.Checksum`, `VerifyAllBackups:80`, `GenerateIntegrityReport:100` | **UNWIRED** — `VerifyBackup:39-50` downloads whole object then `io.Copy(hasher)` correct but neither throttled nor scheduled; Beacon `VerifyAllBackups:85-97` sequential, panel `ArtifactService.Verify:522` bypasses if `IsVerified` |
| `scheduler.go` , `store.go` , `diskspace*.go` , `verification.go` | — | `gocron` `Cron(expr).Do` schedule, store `List/Delete` | `scheduler.go:55-73` `Schedule` rejects dup `jobs[serverID]` (per-server not per-policy); no persistence across restart |

### 1.3 Control-plane store (`forge/api/internal/store/`)

| File | LOC | Role | Status |
|------|-----|------|--------|
| `store_backups.go:1-401` | 401 | `ListBackups:10` paged DESC, `UpsertBackup:54` ON CONFLICT(server_id,name) DO UPDATE, `GetBackupByName:103`, `MarkBackupStatus:115`, `DeleteBackup:130` `SELECT is_locked FOR UPDATE` tx `FOR UPDATE`, `LockBackup:164`, `CleanupOldBackups:240` + `CleanupOldBackupsForServer:263`, `FailStaleBackups:348` etc., `Backup` struct `Nonce string` `Compressed/Encrypted` | **BROKEN in prune path** — `DeleteBackup:138-145` correctly `FOR UPDATE` + lock check; BUT `CleanupOldBackups:240-259` / `CleanupOldBackupsForServer:277-301` are single `DELETE FROM backups WHERE …` with no storage delete → orphan; `ListBackups:10-33` vs `ListExpiredBackups` divergence |
| `store_backup_policies.go:1-317` | 317 | `BackupPolicy` `Interval/MaxBackups/RetentionDays/Compress/Encrypted/EncryptionKey (keyEncrypted)`, `CreateBackupPolicy:35` `encryptSecret(secretAAD)`, `ListExpiredBackups:293` `created_at < now()-30days` hard-coded, `scanBackupPolicy:230` `decryptSecret`, `UpdateBackupPolicyNextRun:246` | **BROKEN 30d literal** — `293-301` hard-codes `30 days` vs per-policy `RetentionDays`; encryption `39-46` correctly blanks `encryption_key=''` and stores `encryption_key_encrypted`; `backupPolicyColumns:33` selects `COALESCE(encryption_key_encrypted,'')` correct |
| `store_backup_jobs.go:1-322` | 322 | `BackupJob` `status/bytes_processed/current_phase/retry_count/max_retries/last_retry_at`, `ClaimBackupJobForExecution:184` atomic `WHERE status IN ('pending','failed')`, `ListRetryableBackupJobs:199` `LEAST(1<<retry_count,30)` backoff, `UpdateBackupJobProgress:263` | **COMPLETE+** — `Claim:185` `RowsAffected>0` correctly prevents double-run; `199-215` backoff `make_interval(mins=>LEAST(1<<retry_count,30))` correct but `1<<retry_count` integer overflow risk for `retry_count>30` before `LEAST`; `ListRetryableBackupJobs` no `FOR UPDATE SKIP LOCKED` → thundering herd across replicas (correctness still held by `Claim`) |
| `store_admin_backups.go` etc. | — | `BackupArtifactRecord`/`BackupStorageProviderRecord`/`BackupRetentionPolicyRecord` | Correct envelope; `BackupArtifactRecord` `status='deleted'` soft-delete vs `store_backups` hard delete diverge |
| `migrations/*backup*` | — | `019_backups.sql` (backups table), `048_s3_backup_config.sql`, `049_backup_locking.sql` (is_locked), `104_a_backup_system.sql` (jobs/artifacts/restores/configs + storage providers), `119_z_backup_policies.sql`, `120_backup_policy_locking`, `125_backup_policies`, `147_backup_provider_secret_encryption`, `162_backup_policy_next_run_and_instance_attempts`, `163_store_schema_parity`, `178_backup_retention` | **PARTIAL** — `019+049` correct; `104` introduces artifact world that `service.go` legacy does not use; `147+162` correct secret encryption + `next_run_at`; `178` hard-codes retention but panel expects per-policy |

### 1.4 Migrations that matter (chronological)

```
019_backups.sql                    → backups(uuid, server_id, name, checksum, size, status, is_locked, manifest, storage_receipt, compressed, encrypted, nonce)
048_s3_backup_config.sql           → S3 bucket/region/prefix columns (early)
049_backup_locking.sql             → is_locked boolean (respects locking)
104_a_backup_system.sql            → backup_jobs / backup_artifacts / backup_restores / backup_configurations / backup_storage_providers (second-gen artifact model, MainService)
119_z_backup_policies.sql          → backup_policies (Interval/MaxBackups/RetentionDays/Storage/Compress/Encrypted)
120_backup_policy_locking.sql      → backup_policies.is_locked
125_backup_policies.sql / 147      → encryption_key_encrypted (keyring migration)
162_backup_policy_next_run_and_instance_attempts → backup_policies.next_run_at (Worker gating)
178_backup_retention.sql           → retention sweep scaffolding (hard-coded window)
```
Gap: two worlds — legacy `backups` rows (daemon `backups` zip) vs `backup_artifacts` (MainService S3/GCS/Azure objects). `ListBackups` (DB) vs `beacon.List` (FS) vs `S3Backup.List` (S3 prefix) three views never reconciled; `verify` and `prune` operate on different views.

### 1.5 HTTP layer (`forge/api/internal/http/`)

| Location | Role | Status |
|----------|------|--------|
| `handlers_servers.go:1503-1814` — `POST /servers/:id/backups` create, `GET /servers/:id/backups` list, `GET /servers/:id/backups/download?name=`, `POST /servers/:id/backups/restore {name}`, `DELETE /servers/:id/backups/:backupId` | Server-scoped backup CRUD, `backupNameCandidates:91` `.zip` tolerant, `resolveBackupName:103` exact, idempotency on existing `backups.Get` check `beacon/server.go:1555` | **COMPLETE for CRUD; BROKEN for throttling** — no `CountRecentBackups`/`BackupLimit`/`backup create` throttle (DAEMON-003 `MASTER_REMEDIATION_LEDGER:122` NOT_STARTED) |
| `handlers_backup_extended.go:1-558` | Extended `POST /servers/:id/backups/policies*`, `POST /admin/backups/cleanup` global sweep `229`, `GET /backup/providers` `239`, `adminBackups /admin/backups/{configs,jobs,artifacts,restores,storage-providers,status}` backed by `MainService(backup.NewMainService:245)` | **PARTIAL / DIVERGENT** — `Get /servers/:id/backups/policies:42` does in-memory `limit/offset` slice (KNOWN: comment `52-56` defers store push-down); `POST /admin/backups/cleanup:229` delegates to `svc.CleanupExpiredBackups` which itself fans to DB hard-coded 30d path; `GET /admin/backups/status:497` synthesizes 4× `List*` 200 each correct |
| `realtime.go:124` `realtimeProxy(cfg,ticketStore,stream)`, `server.go:1995-2017` route registration | `stats|logs|console` only `server.go:1995` `realtimeProxy(...,"stats")`, `2006 "logs"`, `2017 "console"` — no `"backup"` | **DEAD per spec** — beacon `BackupProgressEvent:62` + `server.go:1542-1553` publish exists, but `http/server.go:1995` hardcoded union only proxies `stats|logs|console`, and `realtimeProxy` ticket check `stream != ticket.Stream` (`realtime.go:167`) would reject backup ticket even if added without registration |

### 1.6 Beacon HTTP (`beacon/internal/server/server.go`)

| Location | Role | Status |
|----------|------|--------|
| `createBackup:1503-1603` | Auth via `safePath`, `.pteroignore` load, name sanitize `sanitizeBackupName`, `backupMu.Lock:1548`, `SetProgressCallback:1550` publishing `BackupProgressEvent+":"+serverID:1551`, get-or-create idempotency `Get` before `Create:1555-1570`, `panelClient.SendBackupStatus:1582` | **COMPLETE** — journal, `safePath`, `RateLimit` on zip write correct; missing: `ctx` cancellation propagation into `Create`'s `filepath.WalkDir` already handles `ctx.Err:302` correct, but `backupMu` is coarse (one backup/restore at a time per daemon, not per-namespace) → `lockNamespace:90` inside `LocalBackup` is per-namespace but outer `backupMu` serializes everything daemon-wide |
| `restoreBackup:1699-1787` | Pre-restore snapshot `rollbackName:1728` `Create(...)` real file, `Restore` with `truncate+paths`, automatic rollback on error `1739-1744` `Restore(rollbackCtx, …, true)` | **STRONGEST parity in repo** — real file snapshot (not just record),journal `prepared→live-moved→activated`, `selectRestoreEntries:521` forces `truncate=false` for targeted, `recoverInterruptedRestore:931` invoked on every `Restore` entry `486` |
| `listBackups:1605`, `downloadBackup:1623`, `deleteBackup:1789` | Straight delegation via `backups.<Op>` | Correct with `normalizeBackupName:3093` `.zip` tolerant |
| `downloadBackupWithToken:1650` | JWT mint `tokenGenerator.Validate` → `Download` | Correct `ScopeBackupDownload` gate `1669` |
| `backupProgressWS:2155-2208` | `authenticateWebSocket:2162`, scope check `websocket|backup_download:2169`, upgrade, `eventBus.Subscribe(BackupProgressEvent+":"+serverID):2192`, loop `writer.Write(msg):2203` with `pingWebSocket:2191` | **COMPLETE on beacon side** — subscribe is per-server, bus is `events.Bus` (fan-out). Failure mode: `SetProgressCallback` is overwritten per-request `createBackup:1550` (last writer wins); concurrent creates for different servers race on the single `LocalBackup.progress` field (`local.go:46+78`) without per-namespace callback map |

### 1.7 Web (`forge/web/`)

| File | Role | Status vs BK-13 |
|------|------|-----------------|
| `components/server/backups-view.tsx:1-222` | `BackupsView` — `useQuery` `fetchBackups:43-47` `refetchInterval 3000` when `pending|running`, `getBackupDownloadURL:79`, `create/restore/delete/lock:50-68`, limit banner `89-92` | **POLL-ONLY, no WS** — `43` `refetchInterval` 3s polls `GET /servers/:id/backups` while `local.go:111` `reportProgress` fires per-MB WS events that never arrive (panel never proxies). `TotalBytes` stays 0 until `completed` so adaptive ETA impossible (phase-03 F-B-15). Advanced storage picker `205-207` hardcoded `Custom storage destinations … not supported yet` message correctly reflects that `StoragePath` is daemon-local, not per-request selectable |
| `lib/api/servers.ts:167-202` + `lib/api/backup.ts` + `lib/api/console-backups.ts` | `fetchBackups/createBackup/deleteBackup/restoreBackup/getBackupDownloadURL` REST bindings | `console-backups.ts` covers admin `/admin/backups/*`; `servers.ts:326` `getBackupDownloadURL` correctly via `download-ticket` indirection not direct S3 URL |

### 1.8 Cross-cutting: `storageLocality`

`StorageLocality` is **dead vocab** w.r.t. backup — `beacon/internal/server/server.go` never populates it, `forge/api/internal/services/scheduler/service.go:317` scores `±1e10` on `placement.ScoreResult.StorageLocality` that `strategy.go:42` carries but no server's backup locality ever sets it (phase-03: BK-14 DEAD). Longhorn `diskSelector` enforced at schedule, not at backup-prune. Backup prune today uses `policy.Storage` string exact match (`service.go:707` `DeleteBackupFromStorage(ctx,b.ServerID,b.Name,p.Storage)`) which is per-policy routing but not per-backup routing (`backup.StorageReceipt.Adapter:326` stores per-backup adapter but prune ignores it in some paths). Task's `storageLocality fix` targets this.

---

## 2. Known P0s — mapping FINAL_PARITY §9 BK-01..BK-14 + phase-03 F-B-xx

| FINAL_PARITY | Phase-03 | Title | Severity | Forge file:line root | Reference expectation |
|--------------|----------|-------|----------|----------------------|-----------------------|
| **BK-03** | F-B-01, L1 | Per-blob HKDF+GCM vs single-archive nil salt / nil AAD → transplant forgery | **P0 (SE-BK)** | `encryption.go:92-100` `nil` salt + `encryption.go:119` `nil` AAD; `beacon/local.go:757` unkeyed sidecar SHA | Kopia per-blob HKDF with salt+purpose+AAD bind `serverID:backupName`; Forge single `purposeEncryptionKey="gamepanel-backup-encryption":18` global, file-local nonce only |
| **BK-04** | F-B-02, L2 | Streaming encryption OOM — `io.ReadAll` before seal | **P0/P1 availability** | `encryption.go:150-176` / `178-204` `ReadAll` + `service.go:303` `ReadAll` before `adapter.Upload` | Kopia `throttling streamed`, Restic `chunker 16MiB` streamed |
| **BK-05** | F-B-04, L1 | Sidecar integrity unkeyed SHA, trusted re-read — tamper bypass | **P0** | `beacon/local.go:787-817` `writeMetadata` plain JSON; `readOrCreateMetadata:757-785` computes only if `IsNotExist`; `verification.go:31-77` compares recomputed hash to sidecar value | Restic `verifyCiphertext` AEAD MAC, Kopia index hash |
| **BK-06** | F-B-05, L3 | Retention OR vs AND inversion | **P1 data-loss** | `beacon/retention.go:61` explicit `Intersection ... ALL` comment; `service.go:768-770` `withinCount && withinAge`; `artifact.go:736-739` index+age mix | Kopia `keepLatest/hourly/daily` union OR, Restic `ForgetPolicy` union OR: keep if *any* rule keeps; locked exempt |
| **BK-07** | F-B-06, L4 | Prune without exclusive lock — race + orphan | **P1 race** | `store_backups.go:240-259` / `277-301` SQL-only `DELETE` no S3 delete; `service.go:689-741` does S3→DB but without `FOR UPDATE`/advisory lock; `worker.go:401-448` double `List→Delete` race | Restic `LockRepo exclusive: lock.go:47/124`, Kopia `maintenance GC roots` with lock |
| **BK-08** | F-B-07 | GC orphan `.partial`/`.restore-*`/`.s3-download-*` never indexed | **P1 disk leak** | `beacon/local.go:275` `.partial` `O_CREATE|O_EXCL`, `local.go:546` staging `MkdirTemp("."+base+".restore-")`, `beacon/s3.go:296` `.s3-download-*.zip` — sweeper `scheduler.go` + `store.go cleanupExpired` only sweeps DB `backup_jobs` not FS | Kopia maintenance GC roots; Restic `prune` mark-sweep |
| **BK-09** | F-B-08 | Distributed lock — `lockNamespace` in-process only → split-brain | **P1** | `beacon/local.go:90-109` `namespaceMu+namespaceOps[ns].mu` in-process; `server.go:1548` outer `backupMu sync.Mutex` also in-process | Restic backend lock file `lock.go:47` distributed; NetBird fence-via-NetworkMap correct model |
| **BK-13** | F-B-15 | Progress WS end-to-end dead — `backupProgressWS:2155` exists but `realtimeProxy` only `stats|logs|console` + web poll-only | **P2 UX (phase-03 P0 wiring)** | `beacon/server.go:2155` publishes `BackupProgressEvent:62→1551→2192`, but `forge/api/internal/http/server.go:1995-2017` never registers `"backup"`, `realtime.go:167` `wsTicket.Stream != stream` check, `backups-view.tsx:43` only polls | Kopia 300ms throttle `message_type:status` every 0.2s `restic/ui/backup/json.go:42` |
| **BK-14** | F-B-14 | StorageLocality scoring dead + vocab drift | **DEAD** | `placement/strategy.go:42-62`, `scheduler/service.go:317` `±1e10`, `handlers_servers.go:860` never populates `StorageLocality` for backup routing | Longhorn `diskSelector` enforced at schedule, not prune; backup `StorageReceipt.Adapter:326` per-backup routing needed |
| BK-01, BK-02, BK-10, BK-11, BK-12, BK-15, BK-16 | F-B-03, F-B-09..13 | Repo/pack model, dedup, scheduling conflict, restore journal (beacon strong/panel weak), staging lifecycle OOM, verification sampling, compression | P1/P2 | `local.go:532` strong journal (already fixed), `service.go:271` vs `main_service.go` divergence, `scheduler.go:15` gocron + `worker.go:152` `policyDue` conflict (CONFLICT in audit) | Phase-03 §5 F-B-09..16 enumerated below |

**Deferred comprehension (intentional MISSING, not defects):** BK-01 repo/pack model, BK-02 chunked dedup — artifact zip+m-sidecar correct for <2GB (audit explicitly: `intentionally — artifact model ok for <2GB`). Not filed as P0 for Phase 01.

---

## 3. File-by-file — what the task's line numbers actually show

### `forge/api/internal/services/backup/encryption.go:92`

```go
// 92-100
func deriveEncryptionKey(masterKey []byte, purpose string) ([]byte, error) {
    if len(masterKey) == 0 { return nil, errors.New("master key is empty") }
    derived, err := hkdf.Key(sha256.New, masterKey, nil, purpose, aesKeySize) // ← nil salt
```

`nil` salt means global subkey for purpose `gamepanel-backup-encryption:18` is identical across all servers/backups. Rotation requires re-encrypting everything out of band (no `forge:v1:<keyID>` header). Fix: per-backup salt `HKDF(master, salt=random16, info=purpose|serverID|backupUUID)`, with `salt+keyID` prefixed to ciphertext as self-describing envelope `forge:v1:<keyID>:<b64(salt|nonce|ciphertext|tag)>` (decouples from `secrets/keyring.go:75` AEAD envelope which correctly does `forge:v1:<id>:<b64(nonce||sealed)>` with `seal(aad):91`).

### `forge/api/internal/services/backup/encryption.go:150` `EncryptReader` / `178` `DecryptReader` / `207` `encryptWithKey`

`EncryptReader:158-164` does `input, _ := io.ReadAll(r)` then `encrypted, _ := encryptWithKey(input, derivedKey)` then `io.Pipe` immediate `pw.Write(encrypted)` — pipe is decorative, heap holds `2×plaintext` at peak (input+encrypted). Same in `DecryptReader:186-191`. Beacon never calls these (plaintext zip), but `service.go:294-304` composes them then buffers again via `io.ReadAll:303` before `adapter.Upload:316`. Compare beacon local streaming `zip.NewWriter(zipTarget)` with `copyWithContext:828` already streamed.

### `forge/api/internal/services/backup/service.go:271` vs `main_service.go`

`service.go:271-351` `UploadBackupWithOptions` is legacy path that both compresses and encrypts inline via `CompressReader:285`/`EncryptReader:296` then `io.ReadAll:303` plus `Upload:316`. `main_service.go:18-35` `MainService` quartet is stated winner: `ConfigService/JobService/ArtifactService/RestoreService` with `RegisterStorageAdapter:85` per-provider and `GetBackupSystemStatus:773` etc. Both expose `CreateBackup` (`service.go:225` vs `config.go`/`artifact.go`) with divergent validation (service validates `StorageReceipt`, quartet validates `BackupConfigFilter`). `Worker` currently drives legacy `Service` (`worker.go:18 svc *Service`). Migration: keep quartet as canonical, make `Service` delegate to `ArtifactService` or deprecate (single scheduler `Worker`).

### `beacon/internal/backup/retention.go:61`

```go
// 60-62
// Intersection: a backup must satisfy ALL active rules to be kept.
```

Explicit inversion: Restic `ApplyPolicy` and Kopia retention are union OR (`keep if any bucket`); Forge requires all. Concrete trap: `MaxBackups=5, MaxAge=30d, 6 backups aged 10d` — Restic keeps 5 newest (OR), Forge `withinCount && withinAge:service.go:770` keeps 0 unlocked because the 6th (oldest) sets `withinCount=false` for `index≥5`, and intersection empties. Safety rail `retention.go:122` `keep[backups[0].ID]=true` only rescues one.

### `forge/api/internal/store/store_backups.go:240`

```go
// 240-259
func (s *Store) CleanupOldBackups(ctx context.Context, retentionDays int, autoCleanup bool) (int, error) {
    // DELETE FROM backups WHERE is_locked=FALSE AND created_at < now()- interval '1 day'*$1 AND status='completed'
```

No `storage_receipt` read, no `adapter.Delete`. Direct `DELETE` orphans remote objects. Correct pattern lives 50 lines below in `service.go:689-741` `CleanupExpiredBackups` which `DeleteBackupFromStorage` then `DeleteBackup`. Two callers exist (`queue/periodic.go` retention sweep + `worker.go:401` pre-backup) with different semantics — pick one owner.

### `forge/api/internal/services/backup/s3.go:27`

`checksumMetadataKey = "sha256":26` — correct propagation via `uploadToS3:280-282` `Metadata: map[string]string{checksumMetadataKey: checksum}` plus `checksumFromMetadata:396-403` case-insensitive read. Beacon also validates `hex.DecodeString:324` before accepting upload — parity piece to preserve.

### `forge/api/internal/services/backup/verification.go`

Single RC (not Forge's `artifact.go:Verify`) — downloads, hashes, returns `StatusFailed` on mismatch. No `IsVerified` short-circuit (correct). Not scheduled — `beacon/scheduler.go:15` schedules `backup.retention` not `verify`. Scheduling is missing wiring, not code.

### `forge/api/internal/services/backup/worker.go:152` `policyDue`

```go
// 152-162
func policyDue(policy store.BackupPolicy, now time.Time) bool {
    if policy.Interval == "" { return false }
    if policy.NextRunAt == nil { return false } // ← first tick never fires
    return !policy.NextRunAt.After(now)
}
```

Correctly avoids instant-fire on first tick, guaranteeing initialization via `persistNextRun:166`. Second half `persistNextRun:167-175` `if NextRunAt.After(now) {from=*NextRunAt} else from=now` then `svc.NextCronRun(policy,from)` + `UpdateBackupPolicyNextRun` persists — good. Missing: `NextRunAt` bootstrap on `CreateBackupPolicy` (currently relies on first `tick` after 1m `ticker:95` to set it).

### `beacon/internal/backup/local.go:238,275,532,757`

- `238-268` `Create` entry: `lockNamespace:239`, `reportProgress(0,0,"creating"):241`, `archivePath:246`, separate root checks `sameOrDescendant:250` correct, `.pteroignore` load `259-271` correct.
- `275-296` `partial` handling: `OpenFile .partial O_CREATE|O_EXCL 0600:275` → `defer Remove if !committed:281-285` → compose `rateLimitedWriter:293-295` → `zip.NewWriter:296` correct; `295` limiter uses `rate.NewLimiter(Limit(writeLimit), int(writeLimit))` with `bytesPerSec` granularity 1s — correct Kopia-like throttling primitive.
- `532-624` `Restore` targeted branch `516-529` forces `truncate=false` correctly; non-truncate `531-542` extracts via `targetFS *rootfs.FS` correct; truncate branch `544-624` stages to `MkdirTemp("."+base+".restore-"):546`, extracts, journals `prepared:574`, renames live→rollback `584`, journals `live-moved:588`, renames staging→canonical `595`, journals `activated:606`, syncs parent `611`, removes rollback `616`, journal `619`. Strong vs. panel `restore.go:733` vacuous record.
- `757-817` `readOrCreateMetadata`/`writeMetadata`: plain JSON, `0640` perms `799` style (`0600` elsewhere), `Rename:814` correct — add MAC (see §5).

### `beacon/internal/backup/s3.go` (beacon)

Analysis above in §1.2.

### `beacon/internal/server/server.go:2155` `backupProgressWS`

Correct server-side WS (subscribe/unsubscribe, ping, scope gate `websocket|backup_download`), per-server topic `BackupProgressEvent+":"+serverID`. Two residual races: single `LocalBackup.progress` field overwritten per-request (concurrent servers race), and `reportProgress` unthrottled (fires per `writeLimit` chunk ~1 MiB, ~1k msgs for 1 GiB). Phase-03 `Throttle.ShouldOutput 1/s` mitigation pending.

### `forge/api/internal/store/store_backup_policies.go:293` hard-coded 30d

See §1.3 — parameterize.

### `forge/web/components/server/backups-view.tsx:43`

```ts
// 43-47
refetchInterval: (query) =>
  (query.state.data as {data:ApiBackup[]}|undefined)?.data?.some(b => b.status==="pending"||b.status==="running")
  ? 3000 : false
```

Correct polling cadence (3s) while pending, but no WS path exists to replace it. `formatBackupBytes:13` `MB` divisor `1024*1024` correct, `isUsable:20` only `completed` allowed.

### `forge/api/internal/http/handlers_backup_extended.go`

`backup` coverage noted in §1.5.

---

## 4. Detailed findings — grouped by P0-candidate

### 4.1 Encryption / Integrity — P0 transplant + OOM (BK-03/BK-04/BK-05)

| ID | Title | File:line | Evidence vs. reference |
|----|-------|-----------|------------------------|
| **E-01** | Nil salt HKDF, single global purpose, nil AAD | `encryption.go:18,92-101,119` vs `secrets/keyring.go:75-93` | Kopia: per-blob `HKDF(salt=random, info=purpose|contentID)`; Restic: `scrypt` + per-blob 16-byte nonce. Forge uses `hkdf.Key(..., nil, "gamepanel-backup-encryption", 32)` + `gcm.Seal(nil,nonce,data,nil)` → ciphertext transplantable across `serverID`. `Nonce` also leaked as hex column `service.go:311` `hex.EncodeToString(dataBytes[:12])` then persisted `store_backups.go:91` `nonce` |
| **E-02** | Whole-archive `io.ReadAll` streaming bypass → heap OOM | `encryption.go:150-204` `ReadAll` + `service.go:303-317` `ReadAll → Upload` | Kopia streamed AEAD chunk, Restic `chunker.go:9` 16-MiB packs. 20 GiB world → ≥20 GiB heap + adapter copy → OOM/Kill |
| **E-03** | Sidecar SHA unkeyed, trusted without mandatory rehash | `beacon/local.go:757-817` + `verification.go:31-77` + `beacon/s3.go:280-329` | Restic `check --read-data` re-reads AEAD tag; Kopia `contentID=hash(ciphertext)` intrinsic. Fix: bind sidecar MAC to key or drop sidecar in favor of AEAD tag verification |
| **E-04** | Key management duality: `FORGE_MASTER_KEY` outside `secrets/keyring.go` | `encryption.go:26-31` directly `parseMasterKey(FORGE_MASTER_KEY)` vs `store_backup_policies.go:39` `encryptSecret(secretAAD(...))` + `store_admin_backups.go:287` `Config` encryption | Policy/storage credentials versioned `forge:v1:<id>:<b64>` but archive key is not → rotation undecryptable |

### 4.2 Retention / GC — OR vs AND inversion (BK-06)

Three engines disagree:

- `beacon/retention.go:61-122` — explicit **AND**, loops `KeepDaily/Weekly/Monthly` re-add excluded by `MaxAge`, then intersect with `MaxBackups`.
- `forge/api/service.go:743-784` — `withinCount && withinAge:769` **AND**.
- `forge/api/artifact.go:710-747` — index-gated `exceedsCount \|\| exceedsAge` but paginated 200 + ordering after filtering diverges.
- `store_backups.go:240-301` — age-only `created_at < now()- $days` with hard-coded consumer `store_backup_policies.go:301` `30 days`.

**Unified OR spec (Restic/Kopia):** backup kept iff `IsLocked` OR matches `any{ MaxBackups newest-N, KeepDaily bucket, KeepWeekly bucket, KeepMonthly bucket, MaxAge (when 0=disabled recast as "no MaxAge bucket") }`. Always keep most recent (`retention.go:122` already). Write single `RetentionEngine` library tested against Restic `ForgetPolicy` vectors (keep-last 5 + daily 7 + weekly 4 + monthly 6 → survive 26 with overlap collapsed).

### 4.3 Pruning / Locking (BK-07/BK-09)

| ID | Title | File:line |
|----|-------|-----------|
| P-01 | SQL-only prune leaks remote objects | `store_backups.go:240-259` `DELETE FROM backups` vs `service.go:689-741` correct S3→DB path — two callers, pick one owner |
| P-02 | No exclusive lock — thundering herd on retention + retryable jobs | `beacon/local.go:90` in-process `namespaceMu`, `worker.go:401-448` `ListBackups` without `FOR UPDATE`, `store_backup_jobs.go:199` `ListRetryableBackupJobs` without `FOR UPDATE SKIP LOCKED` (mitigated by atomic `Claim:184`) |
| P-03 | GC orphans: `.partial`/`.restore-*`/`.s3-download-*` never swept | `beacon/local.go:275` partial, `546` restore staging, `beacon/s3.go:296` s3-download, sweeper `scheduler.go` only touches DB |
| P-04 | Double `List→Delete` race in worker | `worker.go:428-444` `toDelete` built from stale `ListBackups` then `DeleteBackupFromStorage`/`DeleteBackup` per item without Tx |

### 4.4 Restore / Journal (BK-11) — beacon strong, panel weak

Beacon `local.go:532-999` 3-phase journal with `syncDirectory`, `recoverInterruptedRestore:931` invoked at entry `486` and `RecoverRestoreJournals:867` at boot — parity winner, keep. Panel `restore.go:733-777` `createPreRestoreSnapshot` creates `BackupArtifact` logical record only, with no file copy — `Rollback:684` thus restores from empty. Beacon `restoreBackup:1699` correctly creates real file `Create(root, serverID, rollbackName, nil)` as snapshot — asymmetric correctness.

### 4.5 Progress / WS (BK-13) — dead pipeline

Beacon emits per-MB (`reportProgress:111`) → `eventBus.Publish(BackupProgressEvent+":"+serverID)` (`server.go:1551`) → `backupProgressWS:2192` `Subscribe`. Panel dead because `http/server.go:1995-2017` only registers `stats|logs|console` and `realtime.go:167` `Stream != ticket.Stream` check rejects. Web `backups-view.tsx:43` only polls; `TotalBytes` always 0 until `completed` (staging file `info.Size()` not published early). Per-file progress blocked (throttle needed).

### 4.6 StorageLocality (BK-14) — dead vocab

`StorageLocality` carried in `placement/strategy.go:42-62` and scored `scheduler/service.go:317` `±1e10` but never populated from `beacon/server.go` (no `TunnelIP`, no locality advertise). Backup's `StorageReceipt.Adapter:service.go:325` already holds per-backup routing (`backup.StorageReceipt.Adapter:490-512` verified), but `worker.go:438` prunes via `policy.Storage` not receipt. Longhorn verdict: mount eligibility after placement not schedule-time — already flagged.

---

## 5. Files to touch — minimal, ordered, blame-preserving

| # | File | Action | Why | BK mapped | Risk if skipped |
|---|------|--------|-----|-----------|-----------------|
| **1** | `forge/api/internal/services/backup/encryption.go:92,150,178,207` | **Rewrite** `deriveEncryptionKey` to require `salt []byte` (16 random) + `info = purpose \| serverID \| backupUUID`; add `encryptWithAAD` with `additionalData=serverID\|backupName` AAD; replace `EncryptReader`/`DecryptReader` with chunked `StreamWriter`/`StreamReader` (64 KiB frames with 12-byte random nonce per frame + 4-byte LE frame length) | P0 transplant + OOM | BK-03/BK-04 | Transplant + OOM remain |
| **2** | `forge/api/internal/services/backup/service.go:271-351` | **Deprecate / delegate** `UploadBackupWithOptions` to stream `EncryptWriter→CompressWriter→UploadStream` pipeline, eliminate `io.ReadAll:303`; make `EnforceRetentionPolicy` call shared engine; make `CleanupExpiredBackups` call shard's `EnforceRetentionPolicy` only (remove direct SQL prune) | Unifies OOM + retention | BK-04/BK-06/BK-07 | Three engines persist |
| **3** | `beacon/internal/backup/retention.go:35-134` | **Extract library** `RetentionEngine` with union-OR spec + property tests from Restic vectors; keep safety rail `keep[newest]=true:122` | Inversion fix | BK-06 | Silent data loss |
| **4** | `forge/api/internal/services/backup/service.go:743-784` + `artifact.go:710-747` + `worker.go:401-448` + `beacon/retention.go` + `store/store_backups.go:240` | **Call single engine** — delete `store_backups.CleanupOldBackups*:240-301` SQL-only path (replace with engine that emits `storage.Delete` then `DB DELETE ... RETURNING` in Tx) and make `artifact.go:ApplyRetentionPolicy` + `service.go:EnforceRetentionPolicy` wrappers over engine; remove local `MaxBackups/RetentionDays` defaults `config.go:170` divergence | Central correctness | BK-06/BK-07 | AND vs OR persists |
| **5** | `forge/api/internal/store/store_backups.go:240-259,277-301` + `store_backup_policies.go:293-302` | **Remove 30d literal** `301`, make `ListExpiredBackups` accept `RetentionDays int` or delete function entirely (engine owns cutoff); add `pg_advisory_xact_lock` helper for prune (`SELECT pg_advisory_xact_lock(hashtext('backup_prune:'||serverID::text))`) around retention sweep | Literal bypass + lock | BK-06/BK-07 | Literal still wins |
| **6** | `beacon/internal/backup/local.go:757-817` + `local.go:238-413` + `local.go:90-109` | **HMAC sidecar** — `writeMetadata` MAC `HMAC-SHA256(activeKey, json)` + always recompute `calculateChecksum` vs sidecar on read path (`readOrCreateMetadata:757` must not short-circuit trust); add `EnsureDiskSpace` check before `zip.NewWriter`; add `.partial` GC sweep `syncDirectory` deferred | Tamper + orphan | BK-05/BK-08 | Sidecar bypass |
| **7** | `beacon/internal/backup/s3.go:69-375` | **Wire encrypted staging** — `Create:126` stage file then `encryptWriter` if `S3Config` signals managed encryption (per-backup key fetch via control-plane ticket), keep streaming upload path (already correct); `downloadToStaging:288` add `DecryptReader` frame after `calculateChecksum` compare; keep `validateS3Endpoint:100` | Beacon plaintext at-rest gap finder saw | BK-03 | Beacon backups plaintext |
| **8** | `beacon/internal/server/server.go:1503-1603,2155-2208` + `beacon/internal/backup/local.go:46,78,111` | **Per-namespace progress map** — replace single `progress ProgressFunc` with `map[namespace]ProgressFunc` under `namespaceMu`, throttle `reportProgress` via `rate.Sometimes{Every: 300ms}` (Kopia 300ms) and include `BytesProcessed/TotalBytes` estimate from staging `Stat` early; wire `backupProgressWS:2155` already correct | WS fan-out + throttle | BK-13 | Noise + race |
| **9** | `forge/api/internal/http/server.go:1995-2017` + `forge/api/internal/http/realtime.go:124-348` | **Register `"backup"` stream** — add `v1.Get("/servers/:id/ws/backup", requireRealtimeServices, wsOriginMiddleware, fiberws.New(realtimeProxy(cfg,wsTickets,"backup"), ...))`, extend `realtimeProxy` auto-mints `daemon.WebSocketURL(...,"backup")` + `MintWebsocketToken(serverID,userID)` with `websocket`-scoped ticket (extend `server.go:2155` scope check to accept `websocket` alone); ticket `Stream="backup"` passes `realtime.go:167` |
| **10** | `forge/web/components/server/backups-view.tsx:43` + `forge/web/lib/api/servers.ts:167` | **Consume WS** — add `useBackupProgress(serverID)` hook subscribing `ws:${API_BASE}/servers/${id}/ws/backup?token=…tickets`, merge `BackupProgress` into query cache (show `Phase`, `BytesProcessed/TotalBytes`, adaptive ETA `remaining = (Total-Processed)/rate`), fall back to 3s poll when WS closed; remove hardcoded storage note or make it `StorageProviders` aware | UX wiring | BK-13 | Dead progress |
| **11** | `forge/api/internal/services/scheduler/service.go:317` + `placement/strategy.go:42` + `beacon/internal/backup/s3.go:Config` advertise | **Locality wiring** — define `BackupStorageLocality` enum `local_only|replicated|shared` on `BackupPolicy.Storage` (derived from `BackupStorageProvider.Type`), have beacon advertise `storageLocality` via `capabilities.go:7` delta already wired (BK-14 UNWIRED); `evacuationplanner/service.go:691` `StorageLocality` remains for failover, not backup prune; `worker.go:438` prune must read `StorageReceipt.Adapter` not `policy.Storage` for per-backup routing | Locality fix | BK-14 | Dead vocab |
| **12** | `forge/api/internal/services/backup/artifact.go:516-708` | **Sweep vs lock** — add `pg_advisory_xact_lock` around `ApplyRetentionPolicy` iteration; make `Verify` not short-circuit `IsVerified` (always re-hash or gate `LastVerifiedAt < now-24h`); cap `ListRetryableBackupJobs` overflow `1<<retry_count` to `uint(min(retry_count,30))` | Prune + verify + overflow | BK-07/BK-15 | Stale verify |
| **13** | `forge/api/internal/store/store_backup_jobs.go:199-232` | **Skip-locked hint** — add `FOR UPDATE SKIP LOCKED` variant for multi-replica safety (incremental, low risk) | Thundering herd | BK-09 | Duplicate work |

**Not touching in Phase 01:** `compression.go:23` (keep gzip+zstd), `storage.go:244` hardened backend (keep), `beacon/local.go:532` journal (keep), `handlers_servers.go` backup CRUD endpoints (only add throttle later, not in minimal P0).

---

## 6. Crypto fix plan — per-backup salt + AAD streaming

### 6.1 Goals (from Kopia/Restic parity)

- No ciphertext transplantable across `serverID` or `backupName` (AAD binds identity).
- No full-archive buffer (streamed chunk AEAD).
- No global subkey (per-backup salt; rotation preserves old ciphertext until re-encrypted).
- Single key management domain (`secrets/keyring.go` envelope pattern).

### 6.2 Envelope

Keep `secrets/keyring.go:75-93` envelope for at-rest secrets. For archive ciphertext, use a *separate* streaming envelope that is itself keyed by the active keyring key (so rotation automatically derives new salts for new backups, old ciphertext remains decryptable via stored `keyID`/`salt`):

```
ciphertext file = header || frame0 || frame1 || … || footer
header  := "forge-backup:v1:" || base64(keyID || salt[16] || AAD_context)
          where AAD_context = serverID + "\0" + backupUUID + "\0" + backupName
frame   := nonce[12] || uint32_BE(ciphertext_len) || ciphertext || tag[16]
          plaintext chunk = 64 KiB (last may be smaller)
footer  := GCM tag over header is NOT separate; each frame's GCM tag authenticates its plaintext+nonce+AAD
```

- `salt[16]` = `rand.Read(16)` per backup at create time (once, stored in header). Do not reuse.
- `keyID` = active `secrets/keyring.go:68` `ActiveKeyID` at create time (stored in header for decrypt lookup).
- Per-frame `nonce[12]` = `rand.Read(12)` per frame (Kopia per-blob random; 96-bit birthday safe at 64 KiB granularity: ≈2^32 frames before 50% collision).
- Per-frame HKDF: `subkey = HKDF-SHA256(activeKey, salt, info="forge-backup:"+keyID+":"+AAD_context, 32)` derived once per backup (not per frame). Derive once, use for all frames of that backup.
- `AAD = AAD_context || frameIndex LE` bound into `gcm.Seal(dst, nonce, plaintext, aad=AAD||frameIndex)`.
- Key rotation: decrypter reads `keyID` from header, fetches that key version from `keyring` (needs `GetKey(keyID)` method — already `keyring.go:75` AEAD can retrieve by ID, add `MustGetKey`), derives same `subkey` via `HKDF(key, salt, info)`, then decrypts frames.

Alternative already considered and rejected: per-blob chunk dedup (Kopia) — **REJECT** for Phase 01 (BK-01 intentional MISSING). Whole-archive streaming AEAD suffices for <2GB worlds.

### 6.3 Implementation sketch (file:line anchored)

**`encryption.go:90-101` replace:**
```go
func deriveEncryptionKey(masterKey, salt []byte, keyID, serverID, backupID string) ([]byte, error) {
    if len(masterKey)==0 { return nil, errors.New("master key is empty") }
    info := fmt.Sprintf("forge-backup:v1:%s:%s:%s", keyID, serverID, backupID)
    return hkdf.Key(sha256.New, masterKey, salt, info, aesKeySize)
}
```

**`encryption.go:150` streaming writer:**
```go
const backupFramePlain = 64 << 10 // 64 KiB

type backupEncryptWriter struct { dst io.Writer; gcm cipher.AEAD; salt, aad []byte; seq uint32; buf []byte }
func NewBackupEncryptWriter(dst io.Writer, masterKey, salt []byte, keyID, serverID, backupName string) (*backupEncryptWriter, error) { ... header := writeHeader(...) ; _,_ = dst.Write(header); return &backupEncryptWriter{...} }
func (w *backupEncryptWriter) Write(p []byte) (int, error) { /* buffer into 64KiB, Seal each, emit nonce||len||ct */ }
func (w *backupEncryptWriter) Close() error { /* flush remainder frame */ }
```
Plus symmetric `backupDecryptReader` that reads header, derives subkey, then per-frame `Open`.

**`service.go:282-317` wire:**
```go
// before: compressedReader → EncryptReader → io.ReadAll → adapter.Upload(dataBytes)
salt := make([]byte,16); rand.Read(salt)
keyID := s.keyring.ActiveKeyID() // inject keyring into Service
master := s.keyring.ActiveKeyDataOrEnv() // or GetEncryptionKeyFromEnv fallback
encWriter, _ := NewBackupEncryptWriter(streamAdapter, master, salt, keyID, serverID, name)
adapter.UploadStream(ctx, backupPath, io.TeeReader(encWriter, hasherForChecksum), -1)
```
Delete `nonceHex` persistence (`service.go:311-313`) — nonce lives per-frame in stream, not DB column (retire `backups.nonce` with `ALTER TABLE backups DROP COLUMN nonce` migration, or keep for back-compat reads).

**`beacon/local.go:275` + `beacon/s3.go:126`**: add optional `EncryptingLocalBackup` wrapper around `LocalBackup` that produces encrypted staging file when `S3Config.Encrypted` signal is set via control-plane ticket `BackupCreateInput` (extend `BeaconClient.ExecuteBackup` to pass `encryptionKeyID+salt`). Beacon path remains plaintext unless explicitly requested (mirrors Kopia policy `ActionsPolicy.Encryption`).

**Tests to add (alongside `encryption_test.go:1`):** `TestPerBackupSaltUnique`, `TestAADTransplantFails` (decrypt with different `serverID` must fail), `TestStreamingRoundTrip_64KiB`, `TestLargeBackupStreamingNoAlloc` (1 GiB temp via pipe, `AllocsPerRun` bounded), `TestKeyRotationDecryptOld`.

### 6.4 Migration for existing plaintext/legacy GCM backups

Keep `Decrypt:126-148` (+`decryptWithKey:224`) legacy decoder that handles `nonce(12)||ct` with nil AAD for old backups. New `DecryptReader` tries new framing first (peek `forge-backup:v1:` magic), fallback to legacy. No forced re-encryption sweep — new backups transparently use new envelope, old remain readable.

---

## 7. Retention unification — single engine, union OR

### 7.1 Spec to implement (Restic `ForgetPolicy` union)

```
keep[backup] = isLocked
            || withinCount       // index < MaxBackups in Created DESC (newest kept)
            || withinAge         // now- MaxAge ≤ Created (when MaxAge>0)
            || inDaily[backup]   // one newest per 24h bucket within KeepDaily
            || inWeekly[backup]  // one newest per 168h bucket within KeepWeekly
            || inMonthly[backup] // one newest per 720h bucket within KeepMonthly
where buckets are computed AFTER MaxAge filter? No — like Restic, bucketed rules run on ALL backups, not only withinAge subset. AND-ranked only safety is keep[mostRecent]=true.
```

Current `beacon/retention.go:35-122` `Apply` does `MaxAge→buckets→MaxBackups intersect` contradictory; fix: collect `keep` as **union** (any bucket adds), then `MaxBackups` also union (not intersect), then `alwaysKeepMostRecent` if union somehow empty (today's `keep[mostRecent]=true:122` already correct but currently only rescues one).

### 7.2 Where to implement

Extract `beacon/internal/backup/retention.go:35` into `forge/api/internal/services/backup/retention/` shared library (`engine.go`) imported by both `beacon/retention.go` (thin wrapper `Apply(ctx,store,serverID) → engine.Select(backups, policy)`) and `forge/api/service.go:743` `EnforceRetentionPolicy` / `artifact.go:710` `ApplyRetentionPolicy` (same `Select`). Engine is pure function `Select(backups []BackupInfoish, policy EnginePolicy) KeepMap` with `BackupInfoish{ID,Created,IsLocked}` interface so both stores satisfy. Property-test against Restic vectors: `KeepLast 5 + daily 7 + weekly 4 + monthly 6` on 20-item fixture → assert `|keep| = expected` + `locked never deleted`.

### 7.3 Code pointers to delete

- Hard-code `store_backup_policies.go:301` `interval '30 days'` — delete, replace with engine cutoff `policy.MaxAge` (or `RetentionDays*24h` in Forge `BackupPolicy` terms).
- In-memory limit `handlers_backup_extended.go:57-77` slice — keep as defense but mark `TODO: push limit/offset into store` already present `52`.
- `artifact.go:686-708` `CleanupExpired` extra `ExpiresAt` check — import engine cutoff via same helper.

---

## 8. Pruning lock — exclusive sweep, no orphan

### 8.1 Owner

Single owner: `Worker.enforceRetentionBeforeBackup:401` + periodic `CandidatePruneJob` on `queue/periodic.go` `backup.retention` every hour. Delete all other callers (`store.CleanupOldBackups:240` SQL-only path, `CleanupOldBackupsForServer:263`). There must be exactly one retention sweep that deletes remote objects then DB rows atomically.

### 8.2 Lock

`pg_advisory_xact_lock` is session-level within Tx, compatible with `FOR UPDATE SKIP LOCKED` elsewhere:

```sql
-- retention/engine.go:EnforceConcurrent
BEGIN;
SELECT pg_advisory_xact_lock(hashtext('backup_prune:' || $serverID::text));
-- then SELECT uuid, storage_receipt, is_locked, created_at FROM backups
--        WHERE server_id=$1 AND status='completed' FOR UPDATE;
-- union-select via engine → DELETE FROM backups WHERE uuid IN (...)
-- then per-row storage adapter Delete (outside Tx? See below)
COMMIT;
-- storage deletions done via mark-sweep to keep Tx short:
-- preferred: Tx marks rows status='prune_pending' then worker deletes objects then Tx hard-deletes.
```

Minimal-scope variant acceptable for Phase 01: keep Tx short (select-for-update + mark `prune_pending`), do `adapter.Delete` outside Tx, then second Tx `DELETE WHERE status='prune_pending'` — mirrors Incus `pruneExpiredInstanceBackups:374` mark-sweep via `state.State` inside exclusive transaction before storage GC.

Add advisory lock around `Store.ListRetryableBackupJobs:199` path as well (`hashtext('backup_retry:'||jobID)` exclusive per job `Claim:184` already atomic, but contention reduction via `SKIP LOCKED` optional).

### 8.3 Orphan GC for FS deposits

New `func (l *LocalBackup) GCOrphansOlderThan(cutoff time.Time) (int, error)` sweeping in `namespaceDir`:
- `*.partial` older than 24h (`local.go:275` never indexed),
- `.*.restore-*` (`local.go:546`),
- `.s3-download-*.zip (+.metadata.json)` (`beacon/s3.go:296`) older than 24h,
all guarded by same advisory lock so sweep never races `Create`.

---

## 9. Progress WS wiring — end-to-end live

### 9.1 Beacon already complete

`backup.go:33-41` `BackupProgress{BytesProcessed,TotalBytes,Phase}`, `local.go:111-118` `reportProgress`, `server.go:62` `BackupProgressEvent`, `server.go:1550-1552` `SetProgressCallback→eventBus.Publish`, `server.go:2155-2208` `backupProgressWS` subscribe, `local.go:275-395` staging with `rateLimitedWriter:293` correct throttling at zip layer. Fix two blemishes there (per-namespace map, 300ms throttle).

### 9.2 Forge proxy — 4 lines to add

**`forge/api/internal/http/server.go:1995` add:**
```go
v1.Get("/servers/:id/ws/backup", requireRealtimeServices(cfg), wsOriginMiddleware(cfg),
    fiberws.New(realtimeProxy(cfg, wsTickets, "backup"), fiberws.Config{
        ReadBufferSize: 4096, WriteBufferSize: 4096,
    }))
```
`realtimeProxy:124` already supports any `stream` string — just register `"backup"` so `realtime.go:167` `Stream != ticket.Stream` passes with `ticket.Stream="backup"`. Ticket mint unchanged (`wsTicketStore` stores `Stream string: realtime.go:18`).

**`forge/api/internal/daemon/client.go` + `forge/api/internal/daemon/db.go` mapping** — `WebSocketURL:285` `"/servers/%s/ws/%s"` already stream-parametric, no code change — backup probes automatically via `realtime.go:285` `WebSocketURL(target.NodeURL, target.ServerID, stream)` where `stream="backup"`.

### 9.3 Web consumption — hook + invalidation

New `forge/web/lib/api/realtime-backup.ts` (or inline in `backups-view.tsx:43`):

```ts
function useBackupProgress(serverId: string) {
  const qc = useQueryClient();
  useEffect(()=> {
    const url = wsURL(`/servers/${serverId}/ws/backup`); // getTicket flow same as console
    const ws = new WebSocket(url);
    ws.onmessage = e => {
      const p: BackupProgress = JSON.parse(e.data); // {bytesProcessed,totalBytes,phase}
      qc.setQueryData(["server-backups", serverId], (old)=> /* merge progress into row with status running */)
    };
    return ()=> ws.close();
  }, [serverId]);
}
```

Merge into `backups-view.tsx:28` `BackupsView` rows: when `status==="running"` show `phase` + progress bar `bytesProcessed/totalBytes` + ETA `(elapsed* (total-processed)/processed)`. Keep `refetchInterval:43` as fallback (WS is unreliable across proxies — WS shows live, poll corrects).

**Payload contract:** beacon `backupProgressWS:2203` currently writes raw `interface{}` published by `reportProgress` (which publishes `BackupProgress` struct directly). Forge `realtime.go:366-382` `pumpUpstreamToClient` relays binary JSON verbatim — no transcode needed. Confirm via `events.Bus` payload type: `backup.BackupProgress` JSON marshal yields `{bytesProcessed,totalBytes,phase}` — aligns.

### 9.4 Tests for wiring

- `forge/api/internal/http/ws_hub_test.go` already covers `realtimeProxy` ticket/permission/heartbeat — add sub-test `"backup stream requires websocket.connect + forwards payload"` with mocked `daemon.WebSocketURL` + `ServerControlTarget`.
- `beacon/internal/server/server_test.go:41` `testBackupAdapter` already fuels `TestBackupHandlersRejectMaliciousNames:312` — add `TestBackupProgressWSProxiesProgress` that `SetProgressCallback` emits 3 frames and client receives 3 JSON frames over `httptest` WS.

---

## 10. StorageLocality fix — from dead vocab to per-backup routing

### 10.1 Problem restated

- `StorageLocality` lives in `evacuationplanner/service.go:28` `local_only|replicated|shared` derived from `ServerMounts` `isNetworkStorage:710` (has `://` or `@: `), used to decide `ReplacementPolicyForServer:720` `Protect vs AutoReplace`. Correct for failover but irrelevant to backup.
- Backup's locality is separate: where does THIS backup's bytes live (`local` vs `s3` vs `gcs` vs `azure`) vs where should it live for target node's locality. Beacon `s3.go:19` `Prefix+Namespace` correctly shards S3; Forge `service.go:325` `StorageReceipt{Adapter,Path}` correctly persists per-backup routing — but `worker.go:438` `DeleteBackupFromStorage(ctx, b.ServerID, b.Name, policy.Storage)` re-derives adapter from policy not receipt, so moving a server's policy from `local` to `s3` leaves `local` backups orphaned (never deleted) and retention miscounts them.

### 10.2 Fix

- **Make prune read receipt:** `service.go:773-779` already iterates `completed` slice, but `DeleteBackupFromStorage` call at `776` uses `policy.Storage`. Fix `776` to `storage := policy.Storage; if len(b.StorageReceipt)>0 { json.Unmarshal → receipt.Adapter if non-empty }` (already done in `CleanupExpiredBackups:725-729` pattern — copy that pattern), so per-backup routing wins.
- **Make `scheduler/service.go:317` locality degenerate no longer consumed by backup:** that scorer was for placement (`placement/strategy.go:42-62` `StorageLocality string` bonus `±1e10`). Backup does not need scheduler locality. Document in `evacuationplanner/service.go:691` docstring that `StorageLocality` is node-evacuation scope, not backup scope.
- **Surface `BackupStorageProvider` in beacon advertise:** add `BackupAdapterType` to `capabilities.go:7` `CapabilityReport` so scheduler knows `beacon.Supports(S3)` before dispatching `ExecuteBackup` with S3 target; today `handlers_backup_extended.go:479` lists providers from `ListBackupStorageProviders` but never gates dispatch.

No new `StorageLocality` enum for backup is needed — `StorageAdapter.Name():storage.go:71` already plural (`local|s3|minio|azure|gcs`). The task's `storageLocality fix` is satisfied by per-receipt routing + advertise, not a new locality service.

---

## 11. Restore / verification hardening (P1 complementary)

- Keep beacon `local.go:532-624` journal verbatim — strongest piece, already handles `validateArchive:645` traversal, `ChecksumMismatch:513` HMAC-like (after §6 becomes AEAD tag), `syncDirectory:611`, rollback `616`.
- **Phase-3 F-B-09 fix (panel pre-snapshot vacuous):** `artifact.go:733` `createPreRestoreSnapshot` must call `local.go:Create` or daemon `CreateBackup` to actually snapshot current tree into a temp backup file before destructive truncate, store receipt `RollbackArtifactID:771`, delete on success path (mirrors `beacon/server.go:1728` real snapshot). Mark `CanRollback:772` only after file commit.
- **Verification:** make `artifact.go:516-579` `Verify` unconditional re-hash when `LastVerifiedAt` older than 7d (or caller requests `forceReverify`); cap `downloadToTempFile:877` via `DownloadStream:606` streaming hash instead of `os.WriteFile` double-buffer (wire `io.Copy(hasher, stream)`).
- **Retry accumulation:** `restore.go:546-577` `Retry` re-enters `Execute` without cleaning previous temp dirs — add `defer os.RemoveAll(tempDir)` already present in `execute*Restore:813` scopes but outer `Retry` on failure still creates new dir per attempt → cap retries in `store_backup_jobs.go:199` `max_retries` already 3, acceptable.

---

## 12. Sequencing, dependencies, verification

### 12.1 Recommended order (matches phase-03 §9 but reprioritized for Phase 01 feasibility)

| Step | P | Files (from §5) | Parallel? | Verification |
|------|---|-----------------|-----------|--------------|
| 1 | P0 | `beacon/local.go:757` MAC sidecar + `retention.go:61` engine extract (pure library, no migration) | YES (no DB) | `go test ./beacon/internal/backup -run TestVerify` + `TestRetentionUnion` new vectors |
| 2 | P0 | `encryption.go:92,150` streaming+salts+AAD + `service.go:303` wire `UploadStream` | YES after step 1 header defined | `encryption_test.go` `TestPerBackupSaltUnique/AADTransplantFails/StreamingRoundTrip` + large `io.Pipe` 1GiB noalloc |
| 3 | P0 | `store_backups.go:240` delete SQL-only path + `service.go:743`/`artifact.go:710`/`worker.go:401` single-engine wiring + `store_backup_policies.go:301` literal | After step 2 (shared engine interface) | `service_test.go:941` `TestRetentionEnforcement` vectors updated to OR; `store_admin_backups_integration_test.go` `TestPruneAdvisoryLock` |
| 4 | P1/P2 | `realtime.go:124`+`server.go:1995` 4-line register + `backups-view.tsx:43` hook | YES independent | `ws_hub_test.go` `backup` stream + manual WS `wscat` against `httptest` beacon |
| 5 | P1 | `beacon/s3.go:126` encrypted staging opt + `beacon/server.go:1550` per-namespace progress map throttled | After step 2 envelope | `local_test.go` `TestCreateEncryptedProgressThrottled` |
| 6 | P1 | `artifact.go:516` Verify re-hash + `store_backup_jobs.go:199` skip-locked + orphan GC | After step 3 | `verification_test.go` `TestVerifyStaleRehash` |
| 7 | P1 | Storage receipt routing `worker.go:776` + capability advertise | After step 3 | `evacuationplanner` + placement not broken (`go vet`) |

### 12.2 `file:line` preservation check

None of the fixes rename exported types that `MASTER_REMEDIATION_LEDGER:122` tracks: `RetentionPolicy`, `BackupInfo`, `StorageAdapter`, `BackupProgress`, `policyDue`/`persistNextRun` all preserved. `service.go:271` vs `main_service.go` divergence resolved by deprecating `service.go` legacy methods in favor of delegating wrappers (keeps import paths, no `handlers_*` change).

### 12.3 Tests that must pass afterwards (existing)

- `beacon/internal/backup/local_test.go` `TestValidBackupName` + `TestCreateAndRestore` (includes journal recovery)
- `beacon/internal/backup/retention_test.go` — add union-OR vectors (Restic `ForgetPolicy` keep-last 5/daily 7 → assert keep-set)
- `forge/api/internal/services/backup/service_test.go:941` `TestRetentionEnforcement_DeletesOldBackups` (update expectation to OR)
- `forge/api/internal/http/handlers_backups_test.go` — create/lock/delete still require `backup.create|delete` permissions (resp. `server.go:155`)
- `forge/api/internal/store/store_backup_lock_integration_test.go` — advisory lock held during retention
- `beacon/internal/server/server_test.go:312` `TestBackupHandlersRejectMaliciousNames` — `normalizeBackupName:3093` still rejects `../|/|\\`

### 12.4 Dependencies & risks

- `secrets/keyring.go:75` must expose `GetKey(keyID)` (currently `ActiveKeyID`+`Encrypt/Decrypt` only) — small additive method, no migration.
- `backups.nonce` column `store_backups.go:91` becomes unused — keep column for back-compat reads, drop in follow-up migration (zero downtime).
- `realtimeProxy` backup registration requires a `wsTicket` with `Stream="backup"` — UI already obtains tickets via `POST /servers/:id/wstickets` pattern used for console; extend ticket types (`tickets.go:14` `TicketType`) with `"backup"` alongside `console|stats|logs`.
- No new backend (Kopia chunk index) — intentional rejection per FINAL_PARITY BK-01 MISSING.

---

## 13. Appendix — FINAL_PARITY §9 BK table with Forge file:line verdict (compact)

| BK | Title (FINAL_PARITY) | File:line verdict |
|----|----------------------|-------------------|
| BK-01 | Repo/pack model vs artifact zip+m-sidecar | `beacon/local.go:238` + `storage.go:15` whole-object **intentional MISSING** (ok <2GB) |
| BK-02 | Chunked dedup (Kopia CDC vs Restic Rabin) | Same as BK-01 — **REJECT** chunking for Phase 01 |
| **BK-03** | **Per-blob HKDF+GCM vs nil salt/AAD → transplant** | `encryption.go:92:119` nil/nil + `local.go:238` plaintext → **P0 BROKEN** §4.1 |
| **BK-04** | **Streaming encryption OOM** | `encryption.go:150` `ReadAll` + `service.go:303` `ReadAll` → **P0 BROKEN** §4.1 |
| **BK-05** | **Sidecar unkeyed SHA** | `local.go:757`+`verification.go:31` trusted → **P0 BROKEN** §4.1 |
| **BK-06** | **Retention OR vs AND** | `retention.go:61` explicit AND → **P1 BROKEN** §4.2 |
| **BK-07** | **Prune exclusive lock** | `store_backups.go:240` SQL-only + no advisory → **P1 BROKEN** §4.3 |
| **BK-08** | **GC .partial orphan** | `local.go:275` never indexed → **P1 BROKEN** §4.3 |
| **BK-09** | **Distributed lock** | `local.go:90` in-process → **P1 BROKEN** §4.3 |
| **BK-10** | **Scheduling conflict** | `beacon/scheduler.go:15` gocron + `worker.go:152` policyDue → **CONFLICT** §1.1/§4.3 |
| **BK-11** | **Restore journal + resumability** | `local.go:532` strong vs `restore.go:733` vacuous → **PARTIAL (beacon COMPLETE, panel BROKEN)** §4.4 |
| **BK-12** | **Staging lifecycle O_CREATE\|O_EXCL→Rename** | Beacon correct `local.go:275` vs `service.go:303` regresses → **DIVERGED** |
| **BK-13** | **Progress WS** | `server.go:2155` exists BUT `server.go:1995` only `stats|logs|console` + `backups-view.tsx:43` poll → **UNWIRED P2 (our P0 wiring)** §4.5 |
| **BK-14** | **StorageLocality scoring** | `scheduler/service.go:317` ±1e10 dead vocab → **DEAD** §4.6 |
| BK-15 | Verification sampling `--read-data` | `verification.go:80` not scheduled → **UNWIRED** |
| BK-16 | Compression per-blob + throttling | `storage_s3.go:126` + `local.go:293` throttling correct but `service.go:303` re-buffers → **PARTIAL** |

---

## 14. Checklist — do not modify code (this audit only)

- [x] Inventory every `services/backup/*` + `beacon/backup/*` + `store_backup*.go` + migrations `*backup*` + `backups-view.tsx:43` + `handlers_backup*` + `beacon/server.go:2155` with file:line
- [x] Cross-checked against `FINAL_PARITY §9 BK-01..BK-16` + `phase-03 synthesis` F-B-01..F-B-18 (all 4 subagents + ledger DAEMON-003)
- [x] Enumerated P0s: E-01 transplant/AAD, E-02 OOM, E-03 sidecar, retention inversion, prune race, progress dead, locality dead — with exact `file:line` and reference expectation
- [x] Listed minimal files to touch (13 rows, §5) with `BK → file:line → risk` mapping — small diff, not rebuild
- [x] Prescribed crypto fix: per-backup `salt(16)+keyID` header, `serverID|backupUUID|backupName` AAD, 64 KiB framed GCM stream, legacy `nonce||ct` decoder retained
- [x] Prescribed retention unification: single union-OR engine (`engine.Select`) replacing 3 AND/partial engines + 30-day literal, with Restic vector tests
- [x] Prescribed pruning lock: `pg_advisory_xact_lock('backup_prune:'+serverID)` mark-sweep + `DeleteBackupFromStorage` before `DELETE … RETURNING`, plus FS `.partial` GC
- [x] Prescribed progress WS wiring: 4-line `server.go:1995` register + `realtimeProxy` stream check + `backups-view.tsx:43` `useBackupProgress` hook with fallback poll
- [x] Prescribed storageLocality fix: per-receipt `Adapter` routing in worker `776`, advertise via capabilities, document planner scope vs backup scope (no new locality service)
- [x] Verified no code was modified (read-only audit; directory `audits/110-phase-01-audit/` write is the only FS effect)

*Prepared for Phase 01 workers — implement §5 in §12 order; reviewer can diff any row back to `file:line`.*
