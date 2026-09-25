# Subagent 04 — FORGE BACKUP ARCHITECTURE vs REFERENCE (KOPIA / RESTIC)

**Dimension:** FORGE BACKUP ARCHITECTURE vs REFERENCE — DUPLICATION / ARTIFACT MODEL / PROVIDER ABSTRACTION  
**Cluster:** kopia, restic  
**Scope:** `forge/api/internal/services/backup` (service.go vs main_service.go, adapters.go, interfaces.go, storage*.go), beacon `internal/backup`, migrations backup retention, HTTP handlers_backup*  
**Date:** 2026-08-23

---

## 1. Executive Summary

Forge's backup subsystem is a layered accretion of two complete implementations that overlap in type definitions, provider factories, local storage backends, retention semantics, and scheduler integration — with neither implementation fully retired. The older monolith `Service` (`service.go:170`) persists alongside the newer facaded `MainService` (`main_service.go:19`) which delegates to four sub-services (`ConfigService`/`JobService`/`ArtifactService`/`RestoreService`). Both expose storage provider abstraction, cron scheduling, and retention enforcement, but with divergent semantics and disjoint runtime wiring.

Compared to reference systems (Kopia, Restic), Forge implements **none** of the content-addressable, deduplicated, incremental architecture. Every backup is a full archive (tar.gz or .zip), compressed and optionally AES-GCM-encrypted as a single blob, stored under a flat namespace. Dedup, content-defined chunking, pack/index layers, and snapshot manifests — the core of Kopia (`repo/content`, `repo/blob`, rolling hash) and Restic (`internal/repository/pack`, snapshot/parents) — are absent. The result: linear storage growth, no cross-backup sharing, and restore that requires full download + full extraction.

Provider abstraction leaks across three parallel registries; local storage has two independent implementations with different security postures; queue coupling is absent (synchronous execution inside the HTTP handler or inside the 1-minute Worker tick); and most of the newer admin API surface has no background runtime to drive it.

**Severity:** HIGH — duplication + divergent retention + insecure local adapter + synchronous execution can cause data loss, path traversal, and request-timeout failures under real backup sizes.

---

## 2. File Inventory

| File | LOC | Role |
|---|---|---|
| `forge/api/internal/services/backup/service.go` | 902 | Old monolith `Service` — CRUD, upload/download, retention, cron parsing, global provider registry |
| `forge/api/internal/services/backup/main_service.go` | 1160 | New facaded `MainService` delegating to 4 sub-services |
| `forge/api/internal/services/backup/config.go` | 613 | `ConfigService` — BackupConfig CRUD, validation, cron next-run |
| `forge/api/internal/services/backup/job.go` | 1099 | `JobService` — job creation, claim, beacon/daemon dispatch, backoff |
| `forge/api/internal/services/backup/artifact.go` | 1061 | `ArtifactService` — artifact CRUD, verify, download, retention, beacon path |
| `forge/api/internal/services/backup/restore.go` | 1292 | `RestoreService` — restore lifecycle, journal, daemon vs beacon split |
| `forge/api/internal/services/backup/storage.go` | 380 | `StorageBackend` + `LocalStorageBackend` (secure) + `StorageManager` |
| `forge/api/internal/services/backup/storage_s3.go` | 938 | `StorageAdapter` + `S3/MinIO/Azure/GCS/LocalStorageAdapter` (insecure local variant) |
| `forge/api/internal/services/backup/adapters.go` | 74 | Thin `*Factory(map[string]string)` wrappers |
| `forge/api/internal/services/backup/interfaces.go` | 146 | `Logger`, `Scheduler`, `BeaconClient`, `StorageAdapter`, `FileInfo` |
| `forge/api/internal/services/backup/worker.go` | 458 | `Worker` — 1-min tick, policyDue, persistNextRun, pickupBackupJobs |
| `forge/api/internal/services/backup/compression.go` | 257 | gzip/zstd pipe helpers |
| `forge/api/internal/services/backup/encryption.go` | 239 | AES-GCM + HKDF (`purposeEncryptionKey`), EncryptReader/Decrypt |
| `forge/api/internal/services/backup/retry.go` | 68 | `withRetry` + jitter |
| `beacon/internal/backup/backup.go` | 163 | `BackupInterface`, `BackupManager`, validation helpers |
| `beacon/internal/backup/local.go` | 1026 | `LocalBackup` — zip creation, journal, legacy migration, symlink-safe |
| `beacon/internal/backup/s3.go` | 414 | `S3Backup` — local staging + S3 upload, checksum, disk reserve |
| `beacon/internal/backup/retention.go` | 134 | `RetentionPolicy.Apply` — intersection + keep-newest safety rail |
| `beacon/internal/backup/scheduler.go` | 150 | `Scheduler` (gocron) — per-server cron, adapter map |
| `beacon/internal/backup/store.go` | 76 | `Store` (backups SQLite), `SQLiteStore` |
| `beacon/internal/backup/verification.go` | 118 | `VerifyBackup`, `GenerateIntegrityReport` |
| `forge/api/internal/http/handlers_backup_extended.go` | 556 | Dual-router: old `Service` routes + new `MainService` admin routes |
| `forge/api/internal/http/handlers_servers.go` | 2000+ | `/servers/:id/backups*` legacy endpoints |
| `forge/api/migrations/019_backups.sql` | 16 | `backups` table (legacy) |
| `forge/api/migrations/104_a_backup_system.sql` | 577 | `backup_configurations` / `backup_jobs` / `backup_artifacts` / `backup_restores` + stub tables |
| `forge/api/migrations/132_backup_schedules_orchestration.sql` | — | Alters `backup_policies` + `backups` (workload-aware) |
| `forge/api/migrations/178_backup_retention.sql` | 24 | `backup_retention` (engine-keyed) |
| `forge/api/internal/store/store_backups.go` | 401 | `backups` CRUD, `CleanupOldBackups`, `FailStaleBackups` |
| `forge/api/internal/store/store_backup_policies.go` | 317 | `backup_policies` CRUD (old policy table) |
| `forge/api/internal/store/store_backup_jobs.go` | 322 | `backup_jobs` CRUD, `ClaimBackupJobForExecution`, `ListRetryableBackupJobs` |
| `forge/api/internal/store/store_admin_backups.go` | 497 | `backup_configurations` / `backup_artifacts` / `backup_storage_providers` / `backup_retention_policies` |

---

## 3. Holistic Comparisons vs Kopia / Restic (15)

### C01 — Artifact model vs Pack/Index model

| Aspect | Forge | Kopia | Restic |
|---|---|---|---|
| Unit | **Artifact** (`artifact.go:21`): one row = one full file (`BackupArtifact` — `storage_provider`, `storage_path`, `file_size`, `file_hash`) | **Content** (deduplicated chunk, BLAKE2/xxhash) packed into **pack blobs** + **index** + **manifest** | **Blob** (chunk, SHA-256) packed into **pack files** + **index** + **snapshot** |
| Manifest | `BackupManifest` (`service.go:75`): flat JSON, 1:1 with artifact, no index | Manifest = snapshot metadata + root object ID, references content IDs transitively | Snapshot = JSON object referencing tree blobs recursively |
| Relation | artifact ← job ← configuration (1:1 chain) | snapshot → manifest → contents → packs (many:many, deduped) | snapshot → tree → blobs → packs (many:many, deduped) |

**Forge gap:** No index, no indirection. Listing requires scanning all artifact rows (`artifact.go:346` loads all records then filters in Go). Kopia/Restic list via index without touching packs.

**Evidence:** `artifact.go:346` `store.ListBackupArtifactRecords(ctx)` then in-memory pagination; `storage_s3.go:221` `List` on S3 does full `ListObjectsV2` with per-object `HeadObject`.

---

### C02 — No deduplication / no content-defined chunking

Forge: every backup re-archives the full server root (`beacon/internal/backup/local.go:298` `filepath.WalkDir` + `zip.NewWriter`), even if 99% of files unchanged. No rolling hash, no chunking, no reference counting.

Kopia: `repo/content` splitter (Buzhash / Rabin) → variable-size contents, deduplicated across snapshots.
Restic: `internal/repository.NewBlob` chunker → SHA-256 dedup → pack reuse, parent snapshots for incremental.

**Impact:** Linear storage growth. A 10 GiB server backed up hourly = 240 GiB/day vs Kopia/Restic ~10 GiB + deltas. Forge's `CleanupExpiredBackups` and `EnforceRetentionPolicy` delete whole artifacts to bound growth — lossy equivalent of GC without dedup benefit.

---

### C03 — No incremental backup / no parent snapshots

Forge: `job.go:499` `executeAppBackup` etc. always create a fresh `BackupJob` with `BackupPending` and call `daemon.CreateBackup` or `beacon.ExecuteBackup` without passing a parent ID. `beacon/local.go:238` `Create` always writes a new `.partial` then `.zip` from scratch.

Kopia: `snapshot.Estimate` reuses previous manifest's `rootEntry` to skip unchanged files; only new/modified contents are uploaded.
Restic: `archiver.Archiver` with `parentSnapshot` — unchanged files reference existing blobs via index.

**Forge consequence:** No `parent` column on `backup_jobs` / `backup_artifacts`; `job.go:200` `CreateFromConfig` fabricates a new name `"%s-%s", config.Name, time.Now().Format(...)` without parent linkage.

---

### C04 — Two complete backup implementations (Service vs MainService)

| | Old (`service.go`) | New (`main_service.go`) |
|---|---|---|
| Type | `Service` (`service.go:170`) | `MainService` (`main_service.go:19`) |
| Construction | `backup.New(store)` (`service.go:179`) | `backup.NewMainService(store, logger)` (`main_service.go:38`) |
| Storage registry | `adapters map[string]StorageAdapter` + global `providerFactories` (`service.go:136`) | `storageAdapters map[string]StorageAdapter` + `artifactService.storageAdapters` (`main_service.go:33`) |
| Cron | `cronParser cron.Parser` (`service.go:176`) | delegates to injected `Scheduler` (`main_service.go:30`) |
| Filter | `EnforceRetentionPolicy` (`service.go:743`) | `ApplyRetentionPolicy` (`artifact.go:711`) + `RetentionPolicy` (new table) |
| Worker | `Worker` (`worker.go:15`) holds `*Service`, not `*MainService` | no worker; `MainService` has no tick loop |

Both define `BackupType` (`service.go:47` / `config.go:30`), `BackupStatus` (`service.go:22` / `job.go:27` via const), `StorageAdapter` (`interfaces.go:86` vs `storage.go:15` `StorageBackend`), and `CreateStorageAdapter` is duplicated between `main_service.go:942` and `adapters.go`.

**Liveness:** `handlers_backup_extended.go:35` registers old `svc *backup.Service` for `/servers/:id/backups/policies`, while `handlers_backup_extended.go:243` instantiates a *separate* `adminSvc := backup.NewMainService(...)` for `/admin/backups/*`. Two live graphs with separate adapter maps, separate default adapters, separate provider persistence. `handlers_apphosting.go:80` creates yet a third `appBackupSvc := backup.NewMainService(...)`.

---

### C05 — Two local storage backends with different security postures

| | `storage.go:44` `LocalStorageBackend` | `storage_s3.go:797` `LocalStorageAdapter` |
|---|---|---|
| Path check | `securePath` (`storage.go:244`): rejects `\`, `\0`, absolute, `..`, evaluates symlinks, enforces `basePath` confinement | none — `filepath.Join(basePath, path)` (`storage_s3.go:820`) |
| Permissions | `0700` dir, `0600` file, `EvalSymlinks` canonicalization | `0755` dir, `0644` file, no symlink check |
| Atomicity | `os.WriteFile` with truncate + `MkdirAll(0700)` | `os.Create` + `io.Copy`, truncates if `size>0` via `Truncate(size)` (sparse bug — see L05) |
| Used by | `StorageManager` (never registered in `MainService`) | `LocalStorageAdapter` (registered via `CreateStorageAdapter("local",...)`) |

`service.go:180` and `main_service.go:944` both call `NewLocalStorageBackend` vs `NewLocalStorageAdapter` depending on code path. Production `LocalStorageAdapter` is reachable by an attacker-controlled `storage_path` without traversal protection.

---

### C06 — Provider registry tripled

1. **Global factory map** (`service.go:136`): `providerFactories map[string]ProviderFactory` + `RegisterProvider`/`GetProvider`/`RegisteredProviders` — used by `handlers_backup_extended.go:238` `backup.RegisteredProviders()` and by nothing else.
2. **Per-service map on `Service`** (`service.go:172`): `adapters map[string]StorageAdapter` + `RegisterAdapter` + `adapter(name)` — populated at startup via `RegisterAdapter` calls.
3. **Per-service map on `MainService` + `ArtifactService`** (`main_service.go:33`, `artifact.go:111`): `storageAdapters map[string]StorageAdapter` duplicated across `MainService` and `ArtifactService` (two copies kept manually in sync via `RegisterStorageAdapter` → `artifactService.RegisterStorageAdapter`).

`StorageManager` (`storage.go:276`) is a fourth registry, never wired to any service.

**Leak:** `MainService.LoadStorageProviders` (`main_service.go:453`) reifies adapters from `backup_storage_providers` rows into its local map, but `Service`'s global `providerFactories` is never populated from DB, so `GET /backup/providers` (which reads `RegisteredProviders`) returns empty after `LoadStorageProviders` has succeeded.

---

### C07 — Queue coupling — synchronous execution, dual dispatch, no queue

| Layer | Mechanism |
|---|---|
| `JobService.Execute` (`job.go:327`) | atomic `ClaimBackupJobForExecution` → `switch job.JobType` → if `daemonClient != nil` take **direct daemon path** (sync HTTP call, no queue), else if `beaconClient != nil` do **beacon task path** (async poll + `waitForBeaconTaskCompletion` 30 min) |
| `ConfigService.Execute` (`config.go:401`) | creates job then calls `jobService.Execute` inline |
| `MainService.ExecuteBackupConfig` (`main_service.go:136`) | delegates to `configService.Execute` → same inline path |
| HTTP handler `POST /admin/backups/jobs` (`handlers_backup_extended.go:331`) | `CreateBackupJob` then `ExecuteBackupJob` **synchronously inside the request** (`handlers_backup_extended.go:344`) — blocks HTTP worker for up to 30 min |
| `Worker.pickupBackupJobs` (`worker.go:182`) | polls `ListRetryableBackupJobs(limit=5)` then calls `jobSvc.Execute` **synchronously** inside the 1-min tick loop (`worker.go:192` `runCtx 20min`) — blocks tick for up to 100 min |

No message queue, no `backup_jobs` state machine consumed by a background queue worker. The tick loop and the HTTP handler contend on the same `ClaimBackupJobForExecution` row lock without coordination beyond the DB `status IN ('pending','failed')` filter.

Kopia: `scheduler` + `maintenance` loop decoupled from API; Restic: no scheduler (external cron) — Forge conflates both into the same synchronous path.

---

### C08 — Scheduler — three independent schedulers, none unified

| Scheduler | Trigger | Scope | State |
|---|---|---|---|
| `Worker` (`worker.go:89`): `time.NewTicker(time.Minute)` + `policyDue` + `persistNextRun` | old `backup_policies` (`BackupPolicy` with `Interval` cron string) | old `Service` | reads `ListAllEnabledPolicies`, writes `next_run_at` |
| `ConfigService` (`config.go:212`): `calculateNextCronRun` on create/update | `backup_configurations` (`BackupConfig` with `CronExpression`) | new `MainService`/`ConfigService` | no tick — `NextRunAt` computed but never polled |
| `beacon/internal/backup.Scheduler` (`beacon/scheduler.go:15`): `gocron.Scheduler` wrapping `gocron.Cron(cronExpr).Do(adapter.Create)` | per-server beacon-local cron (`Scheduler.Schedule`) | beacon daemon | separate `adapters map[string]BackupInterface`, never receives `backup_configurations` |
| `MainService.ScheduleBackup` (`main_service.go:292`) | `BackupConfig` cron update + `s.scheduler.ScheduleBackupJob` via `Scheduler` interface | `MainService` | writes `CronExpression` + `NextRunAt` on config, then calls `Scheduler.ScheduleBackupJob` — but no production `Scheduler` implementation is ever registered (`SetScheduler` is optional, nil-guarded) |

**Result:** Creating a `BackupConfig` with `is_scheduled=true` (`config.go:212`) computes `NextRunAt` but nothing ever fires it unless an external caller invokes `MainService.ExecuteBackupConfig` manually. The only auto-firing scheduler (`Worker`) watches the *other* table (`backup_policies`).

---

### C09 — API surface — split across two service graphs, orphaned types

| Endpoint group | Service | Runtime |
|---|---|---|
| `GET/POST /servers/:id/backups/policies` (`handlers_backup_extended.go:40`) | old `Service` (`svc *backup.Service`) | `Worker` tick drives it |
| `GET /servers/:id/backups/*` (`handlers_servers.go:1912`) | legacy `svc` (same `Service`) | no job integration — direct `daemon` calls |
| `POST /admin/backups/configs\|jobs\|artifacts\|restores` (`handlers_backup_extended.go:254`) | *new* `adminSvc := backup.NewMainService(...)` (ephemeral per-router) | **no worker, no queue** — `Execute` runs inline |
| `POST /apps/:id/backups` (`handlers_apphosting.go:721`) | *another* `appBackupSvc := backup.NewMainService(...)` | same — no shared state |
| `GET /backup/providers` (`handlers_backup_extended.go:237`) | global `RegisteredProviders()` | empty unless factories registered |

Types exposed but never served: `BackupConfigFilter.Scheduled`, `ArtifactFilter.IsLocked`, `RestoreFilter.TargetServerID` — `ArtifactService.List` and `RestoreService.List` filter in Go (`artifact.go:353`, `restore.go:278`) after loading all rows, not via SQL.

vs Kopia: `kopia snapshot create`, `kopia maintenance`, `kopia policy`; Restic: `restic backup`, `restic forget --prune` — single coherent CLI/API per repository.

---

### C10 — Retention — four policy tables, three semantics

| Table | Enforcer | Semantics | Scope |
|---|---|---|---|
| `backup_policies.retention_days` + `max_backups` | `Service.EnforceRetentionPolicy` (`service.go:743`): keep if `IsLocked \|\| (withinCount && withinAge)` — **AND** | per-server (`server_id`) | `Worker.enforceRetentionBeforeBackup` + manual `CleanupExpiredBackups` |
| `backup_policies.retention_days` (also) | `Worker.enforceRetentionBeforeBackup` (`worker.go:401`): explicit pre-backup eviction of oldest unlocked exceeding `MaxBackups` — **count-only** pre-delete + same `EnforceRetentionPolicy` | per-server | `Worker.executePolicy` pre-step |
| `backup_artifacts` via `backup_retention_policies` | `ArtifactService.ApplyRetentionPolicy` (`artifact.go:711`): `exceedsCount = index >= MaxBackups`; `exceedsAge = CreatedAt.Before(cutoff)`; delete if `(exceedsCount \|\| exceedsAge) && !IsLocked` — **OR** | global/server/app/database/volume (`RetentionPolicy.Scope`) | admin `ApplyRetentionPolicy` |
| `backups` via `backup_retention` | `store.CleanupOldBackups` / `CleanupOldBackupsForServer` (`store_backups.go:242`): SQL `DELETE WHERE is_locked=FALSE AND status='completed' AND (created_at < now()-days OR uuid IN (oldest over limit))` — **OR** in SQL | engine-keyed (`backup_retention.kind`) | hourly worker (not wired to `Worker`) |
| Beacon `RetentionPolicy` | `beacon/retention.go:35`: intersection of MaxAge + KeepDaily/KeepWeekly/KeepMonthly + MaxBackups, always keep newest | per-server (`serverID`) | beacon-local, never called from Forge |

**Divergence example (logic bug, see L01):** With `MaxBackups=5, RetentionDays=30`, 10 backups all 10 days old, 6th-newest: old enforcer keeps it (`withinCount=false && withinAge=true → false → delete`), new enforcer also deletes (`exceedsCount=true → delete`), but for 6th-newest that's 20 days old with `MaxBackups=10, RetentionDays=30` — old keeps (`withinCount=true && withinAge=true → keep`), new keeps (`exceedsCount=false && exceedsAge=false → keep`) — consistent. Divergence appears when *one* dimension passes and the other fails: old requires **both**, new requires **either** fail to delete — the `&&` vs `||` flip causes old to delete more aggressively when count exceeded but age still within window.

---

### C11 — Encryption — per-backup AES-GCM/HKDF vs per-content AEAD

| | Forge | Kopia | Restic |
|---|---|---|---|
| Key derivation | `deriveEncryptionKey(masterKey, "gamepanel-backup-encryption")` via HKDF-SHA256 (`encryption.go:92`) — single purpose string, no per-content salt | `repo/encryption` — per-content key derived from master + content ID | `internal/crypto` — random `encryptionKey` + `macKey`, repository-wide, authenticated |
| Scope | whole-archive `Encrypt(data, key)` → `nonce(12) \|\| GCM(data)` (`encryption.go:119`) | per-content ID (`ecc`, `format`) | per-blob AES-256-CTR + Poly1305 |
| Storage | `Nonce` column hex of first 12 bytes of ciphertext (`service.go:312` `hex.EncodeToString(dataBytes[:nonceSize])`) — **leaks nonce** | nonce inside ciphertext header, not stored separately | nonce inside pack header |
| New path | `ArtifactService` never encrypts — `CreateFromBackupResult` (`artifact.go:248`) stores `IsEncrypted` flag from beacon result but no crypto call; beacon `LocalBackup.Create` never encrypts | — | — |

Kopia/Restic encrypt before upload and verify MAC on every read; Forge's `VerifyChecksumFromStore` (`service.go:451`) re-downloads and SHA-256 compares, **not** AEAD verification — tampered ciphertext that still decompresses would pass if checksum check omitted.

---

### C12 — Compression — per-archive gzip/zstd vs per-content compression

Forge: `compression.go:63` `CompressReader` pipes through `gzip.NewWriterLevel` or `zstd.NewWriter` based on `BACKUP_COMPRESSION_ALGORITHM` env; `service.go:283` does `if opts.Compress { data = CompressReader(data, path) }` then `if len(opts.EncryptionKey)>0 { data = EncryptReader(data, key) }` — always compress-then-encrypt. Beacon's `LocalBackup.Create` (`local.go:297`) uses `zip.Deflate` unconditionally, regardless of `Compress` flag — second compression layer. `ArtifactService` stores `IsCompressed` but delegates to `beacon` which always compresses — flag is informational only.

Kopia: per-content compression (`repo/compression`, `zstd/gzip/s2`) with `compressible` detection, encrypted after compression, verified via content hash.
Restic: no compression — relies on external.

**Bug surface:** `Decompress` vs `DecompressReader` branch on same env `getCompressionAlgorithm()` — if env changes between upload and download, decompression fails. No algorithm stored per-artifact beyond `CompressionAlgorithm *string` (often nil).

---

### C13 — Checksum / integrity — SHA-256 over whole archive vs per-blob hash chain

Forge: `calculateChecksum` (`beacon/backup.go:98`) SHA-256 of whole `.zip` file; stored as `localMetadata.Checksum` (`local.go:27`) and `store.Backup.Checksum`; verified via `VerifyChecksum` (`service.go:438` `sha256.New`, `hex`, `EqualFold`), `VerifyStorageReceipt` (`service.go:491` re-downloads full file), `VerifyBackup` (`beacon/verification.go:31` streams download), and `ArtifactService.Verify` (`artifact.go:516` downloads to temp file then `calculateFileHash`).

Kopia: `hashing` of each content (BLAKE2b-256), then hmac on pack; index covers all.
Restic: SHA-256 per blob, pack hash, repository integrity via `prune`/`check`.

Forge re-downloads the entire archive to verify (costly, streaming not chunked); `VerifyStorageReceipt` downloads full blob even when only existence check needed. Kopia/Restic verify via index + spot-check, not full re-download per verify.

---

### C14 — Restore — journal + truncate swap vs mount-based restore

Forge `LocalBackup.Restore` (`local.go:468`): 
- `validateArchive` + `calculateChecksum` pre-check,
- `targeted` restore (`selectRestoreEntries` `local.go:731`) disables `truncate`,
- non-truncate: `extractArchive` directly,
- truncate: staging dir + `restoreJournal` (`local.go:532` `Staging`, `Rollback`, `Phase` = `prepared`/`live-moved`/`activated`), atomic `Rename(live→rollback)` then `Rename(staging→live)`, `RecoverRestoreJournals` (`local.go:867`) on startup.

Forge `RestoreService` (`restore.go:354`): duplicates journal phases (`journaling restore intent` → `activating restore`), adds `createPreRestoreSnapshot` (`restore.go:733`) that creates a fresh `BackupArtifact` as rollback placeholder — but `daemon` path (`restore.go:381` `if s.daemonClient != nil`) short-circuits to `daemon.RestoreBackup` and skips journal/verify entirely via `completeDirectRestore` (`restore.go:503`).

Kopia: `kopia restore` mounts via `kopia mount` or streams via `repo/open`.
Restic: `restic restore` streams from pack+index, selective paths via `filepath.Match`.

**Gap:** Forge's daemon bypass loses crash-consistency guarantee; `RestoreService.execute*Restore` each re-implement temp dir + download + journal + beacon dispatch with copy-pasted `waitForBeaconTaskCompletion` (two copies: `job.go:975`, `restore.go:1262`).

---

### C15 — Migrations / schema — backup table proliferation

| Table | Introduced | Used by |
|---|---|---|
| `backups` (`019_backups.sql`) | legacy | old `Service`, `Worker`, `store_backups.go` |
| `backup_policies` (`057_backup_policies.sql`, extended `132_backup_schedules_orchestration.sql`, `119_z_*.sql`, `125_*.sql`, `162_*.sql`) | old scheduler | `Service`, `Worker`, `store_backup_policies.go` |
| `backup_configurations` / `backup_jobs` / `backup_artifacts` / `backup_restores` / `backup_retention_policies` / `backup_storage_providers` (`104_a_backup_system.sql`) | new admin system | `ConfigService`, `JobService`, `ArtifactService`, `RestoreService`, `MainService` |
| `backup_retention` (`178_backup_retention.sql`) | engine-keyed retention for `managed_database_backups` | `store.CleanupOldBackups` — separate cron |
| `backup_manifests`, `backup_storage_receipts`, `database_backups`, `volume_backups` (`132_*`) | workload-aware extension | only `store.UpsertBackup` manifest/receipt fields, never queried via `MainService` |
| Stub tables `apps`, `databases`, `volumes`, `encryption_keys` (`104_a_backup_system.sql:5`) | FK targets | clash with real `servers`, `applications`, `server_databases` tables |

No migration deprecates `backup_policies` after `backup_configurations` supersedes it; both tables are written and read concurrently.

---

## 4. Logic Findings (4)

### L01 — HIGH — Triple retention semantics diverge (AND vs OR vs Intersection)

**Files:** `service.go:743` `EnforceRetentionPolicy`, `artifact.go:711` `ApplyRetentionPolicy`, `beacon/retention.go:35` `RetentionPolicy.Apply`, `worker.go:401` `enforceRetentionBeforeBackup`, `store_backups.go:242` `CleanupOldBackups`

**Detail:**

Old `Service.EnforceRetentionPolicy` (`service.go:767`):
```go
withinCount := policy.MaxBackups <= 0 || index < policy.MaxBackups
withinAge   := policy.RetentionDays <= 0 || now.Sub(backup.CreatedAt) <= RetentionDays*24h
keep[backup.Name] = backup.IsLocked || (withinCount && withinAge) // AND
```

New `ArtifactService.ApplyRetentionPolicy` (`artifact.go:736`):
```go
exceedsCount := policy.MaxBackups > 0 && index >= policy.MaxBackups
exceedsAge   := !cutoff.IsZero() && artifact.CreatedAt.Before(cutoff)
if artifact.IsLocked || (!exceedsCount && !exceedsAge) { continue } // keep iff neither exceeds
// delete if exceedsCount || exceedsAge                          // OR
```

Truth table identical (De Morgan), so these two are **not** divergent from each other — audit corrects initial hypothesis. True divergence is with **beacon** and **pre-backup count-only** path:

- Beacon (`retention.go:60`): `keep = MaxAge ∩ MaxBackups` (intersection via `delete(keep,id)` loop), then `KeepDaily/KeepWeekly/KeepMonthly` **union** into `keep`, then `MaxBackups` intersection again — **OR across periods, AND with MaxBackups/MaxAge**.
- `Worker.enforceRetentionBeforeBackup` (`worker.go:420`): pre-delete `overLimit = countCompleted - MaxBackups + 1` oldest unlocked regardless of age — **count-only, ignores RetentionDays** in the pre-delete phase, then calls `EnforceRetentionPolicy` (AND) — two-phase double-enforcement deletes more than either alone.

With `MaxBackups=5, RetentionDays=30, KeepDaily=2, KeepWeekly=1` and 10 backups of mixed ages, Forge `Service` keeps `IsLocked || (withinCount && withinAge)`, beacon keeps `(MaxAge ∩ MaxBackups) ∪ KeepDaily ∪ ... ∩ MaxBackups`, producing different survivor sets. A policy test expecting beacon semantics executed through Forge will under- or over-delete.

**Repro sketch:**
```
create 10 backups: 1 per day over 10 days, all unlocked
policy MaxBackups=5, RetentionDays=30, KeepDaily=5
Forge keeps newest 5 (withinCount && withinAge for i<5)
Beacon keeps up to 5 from 0-24h + 5 from 24h-7d + ... ∩ MaxBackups → keeps newest 5 but also may resurrect older ones via KeepWeekly
```

**Fix:** unify on one retention engine; remove `enforceRetentionBeforeBackup` pre-delete; make `ArtifactService` sort explicitly before applying `index`-based `exceedsCount` (currently `artifact.go:351` loads all records then post-filters in Go without sort — `ListBackupArtifactRecords` is `ORDER BY created_at DESC`, but `ApplyRetentionPolicy` loads via `s.List` which reorders via `store.ListBackupArtifactRecords` → DESC, then applies `index >= MaxBackups` where `index 0` is newest — correct by accident, but `List` pagination in `ApplyRetentionPolicy` is hardcoded `Page:1, PerPage:200` then post-filter, so with >200 artifacts deletion is incomplete).

---

### L02 — HIGH — Storage path suffix mismatch causes download/delete misses

**Files:** `service.go:276` `UploadBackupWithOptions`, `service.go:363` `DownloadBackupWithOptions`, `service.go:422` `DeleteBackupFromStorage`, `beacon/local.go:128` `validBackupName`

**Detail:**

Upload:
```go
// service.go:277
backupPath := name
if !strings.HasSuffix(backupPath, ".tar.gz") && !strings.HasSuffix(backupPath, ".zip") {
    backupPath = backupPath + ".tar.gz"
}
```

Download / Delete:
```go
// service.go:363
backupPath := name
if !strings.Contains(backupPath, ".") {
    backupPath = backupPath + ".tar.gz"
}
// service.go:423 identical Contains check
```

Cases:

| `name` | Upload path | Download path | Match? |
|---|---|---|---|
| `backup-01` | `backup-01.tar.gz` | `backup-01.tar.gz` | yes |
| `backup.test` | `backup.test.tar.gz` | `backup.test` | **no** |
| `backup.tar.gz` | `backup.tar.gz` | `backup.tar.gz` | yes |
| `backup.zip` | `backup.zip` | `backup.zip` | yes |
| `my.backup.01` | `my.backup.01.tar.gz` | `my.backup.01` | **no** |

Any name containing a dot but lacking the canonical suffix (e.g., `daily.2026-01-01`) uploads to `daily.2026-01-01.tar.gz` but download looks for `daily.2026-01-01` — `Exists` false, `Download` 404, `Delete` silently misses the object. `VerifyStorageReceipt` (`service.go:513` `s.buildStoragePath(serverID, name)`) uses a third convention `backups/<serverID>/<name>` (no suffix logic), compounding the mismatch.

Beacon side `validBackupName` (`backup.go:125`) requires `.zip` suffix strictly — a Forge-uploaded `.tar.gz` would be rejected by `LocalBackup.Get`/`Delete`.

**Fix:** single `storagePath(name)` helper used by every path; enforce suffix via `HasSuffix` consistently; store canonical `storage_path` on write and never recompute.

---

### L03 — HIGH — Synchronous execution inside HTTP handler / Worker tick; no queue, no backoff persistence, no concurrency guard

**Files:** `handlers_backup_extended.go:344` `POST /admin/backups/jobs`, `handlers_backup_extended.go:457` `POST /admin/backups/restore`, `worker.go:182` `pickupBackupJobs`, `job.go:327` `Execute`, `store_backup_jobs.go:184` `ClaimBackupJobForExecution`, `store_backup_jobs.go:199` `ListRetryableBackupJobs`

**Detail:**

1. **HTTP handler blocks on backup.** `handlers_backup_extended.go:344`:
   ```go
   job, _ := adminSvc.CreateBackupJob(ctx, request, actor)
   if err := adminSvc.ExecuteBackupJob(ctx, job.ID, actor); err != nil { // sync
       return fiber.NewError(500, ...)
   }
   ```
   `JobService.Execute` → `executeAppBackup` (`job.go:500`) → `daemon.CreateBackup` or `beacon.ExecuteBackup` → `waitForBeaconTaskCompletion` 30 min poll (`job.go:975` with `backoff*=2` up to 30s). The Fiber handler holds the request goroutine for the entire backup. With a 10 GiB archive, upload + staging exceeds typical proxy timeout (30–60s) → 504 with orphaned job stuck in `running` (no `defer` marks `failed` on context cancel in the direct daemon path — `job.go:530` `CreateBackup` result stored but no `Claim` rollback).

2. **Worker tick blocks on backup.** `worker.go:182`:
   ```go
   for _, job := range jobs { // limit 5
       runCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
       err := w.jobSvc.Execute(runCtx, job.ID, "scheduler") // sequential, 20m each
       cancel()
   }
   ```
   Sequential execution inside `tick()` (called from `loop` `ticker.C` `worker.go:105`). With 5 jobs × 20 min = 100 min > 1-min ticker period. `loop` does not gate re-entry; but `ticker.C` deliveries queue in the `select` — next tick runs immediately after the long tick completes, so `policyDue` sweep may skip `NextRunAt` windows that elapsed during the blocked tick. `Health` (`worker.go:83`) returns `lastTick` only after `tick` returns — monitoring sees stale `lastTick` during long backups.

3. **No queue, CLAIM is best-effort.** `ClaimBackupJobForExecution` (`store_backup_jobs.go:184`) is atomic:
   ```sql
   UPDATE backup_jobs SET status='running', ... WHERE id=$1 AND status IN ('pending','failed')
   ```
   If the handler crashes after `Claim` but before `CompleteBackupJobSuccess` / `FailBackupJob`, job stays `running` forever — `ListRetryableBackupJobs` (`store_backup_jobs.go:199`) only selects `status IN ('pending','failed')` so `running` jobs are never retried. `FailStaleBackups` (`store_backups.go:348`) only covers `backups` table, not `backup_jobs`.

4. **Backoff split.** `retry.go:34` `withRetry` does jittered exponential backoff per-storage-call; `ListRetryableBackupJobs` SQL does `last_retry_at < now() - make_interval(mins => LEAST(1<<retry_count,30))` (DB-level backoff). `JobService.Execute` also does manual `RetryCount++` + `FailBackupJob(status='pending')` inline (`job.go:381`) with immediate re-queue — three backoff layers with no shared state, causing either tight retry loops (inline `withRetry` retries 3× 500 ms–30s) then DB-level 2^retry minutes, or double-counted retries.

**Fix:** introduce async job queue (e.g., `pgboss`, `river`, or simple `LISTEN/NOTIFY` + worker pool); handler enqueues and returns 202 with `Location`; Worker consumes one job at a time with leasing; add `running` → `failed` reaper via `updated_at < now()-interval` on `backup_jobs`.

---

### L04 — MEDIUM — Encryption + compression pipe + nonce extraction bug; stale algorithm env

**Files:** `service.go:271` `UploadBackupWithOptions`, `compression.go:63` `CompressReader`, `encryption.go:150` `EncryptReader`, `storage_s3.go:589` `GCS Upload` (separate issue)

**Detail:**

```go
// service.go:283
if opts.Compress {
    compressedReader, _ := CompressReader(data, backupPath)
    data = compressedReader // pipeReader
}
if len(opts.EncryptionKey) > 0 {
    encReader, _ := EncryptReader(data, opts.EncryptionKey) // ReadAll(compressed pipe)
    data = encReader
}
dataBytes, _ := io.ReadAll(data) // drains encrypted pipe
if encrypted && len(dataBytes) >= nonceSize {
    nonceHex = hex.EncodeToString(dataBytes[:nonceSize]) // nonce stored separately
}
```

`CompressReader` (`compression.go:77`): `pr, pw := io.Pipe(); gw := gzip.NewWriterLevel(pw,level); go{ io.Copy(gw,r); gw.Close(); pw.Close() }` — returns `pr`.  
`EncryptReader` (`encryption.go:150`): `input, _ := io.ReadAll(r)` — reads the **entire** compressed pipe into memory, then `encryptWithKey(input, derived)` → `nonce||ciphertext`, then wraps in another `Pipe`.  
`io.ReadAll(data)` then reads the encrypted pipe's output into `dataBytes` for upload.

Issues:

- **O(n) memory** twice: `data` fully buffered in `EncryptReader` (`io.ReadAll(r)`  line 158) and again in `io.ReadAll(data)` line 303 — 2× archive size in RAM. A 5 GiB backup OOMs the API pod. Kopia/Restic stream via `hashing.Writer` + chunker without full buffering.

- **Nonce stored but never used.** `nonceHex` extracted as `hex(dataBytes[:12])` and stored on `UpsertBackup` `Nonce` field, but `Decrypt` (`encryption.go:126`) takes full `data []byte` and slices `nonce := data[:12]` itself — the stored `Nonce` column is never read by any decrypt path (`DownloadBackupWithOptions` `decrypt(data, key)` ignores `Nonce`). If decrypt ever needs the stored nonce (e.g., streaming decrypt), the hex-encoded copy is redundant and wrong if encryption key derived via HKDF changes (HKDF uses fixed `purposeEncryptionKey` — same for all backups, but `FORGE_MASTER_KEY` rotation would invalidate stored nonces without versioning).

- **Algorithm env race.** `getCompressionAlgorithm()` (`compression.go:23`) reads `BACKUP_COMPRESSION_ALGORITHM` per-call; `Decompress` (`compression.go:223`) reads env again at download time. If env flips from `gzip` to `zstd` between upload and download, `Decompress` picks the wrong algorithm and fails. Kopia stores `compressionAlgorithm` per-content; Forge's `Backup.Compressed bool` (`service.go:67`) is a bool, not an algorithm name — `CompressionAlgorithm` exists only on `BackupArtifact` (`artifact.go:46`), not on legacy `Backup`.

**Fix:** streaming encrypt without double buffering via `cipher.Stream` or chunked AEAD; store `compression_algorithm` (string) and `encryption_nonce` (binary) per backup; never rely on env at read time.

---

### L06-07 (Additional logic observations)

**L05 — `LocalStorageAdapter.UploadStream` sparse preallocation bug** (`storage_s3.go:831`): 
```go
file, _ := os.Create(fullPath)
if size > 0 { file.Truncate(size) } // creates sparse file of requested size
io.Copy(file, reader)              // appends past the preallocated region → file size = size + len(reader)
```
vs `storage.go:189` `LocalStorageBackend.UploadStream` does same. Restic/Kopia never pre-truncate then append. Fix: remove `Truncate` or `Seek(0)`.

**L06 — `GCSStorageAdapter.List` leaks prefix** (`storage_s3.go:659`): `prefix := a.prefix; if objectPrefix != "" { prefix = objectKey(objectPrefix) }` — when `objectPrefix==""`, raw `a.prefix` (no trailing slash) is sent as `prefix` query param, matching objects with that prefix as substring (e.g., prefix `backups` matches `backups-old/...`). `S3StorageAdapter.List` (`storage_s3.go:221`) uses `a.key(prefix)` with same no-slash issue.

---

## 5. Duplication / Dead Code

### 5.1 Duplicated Types

| Type | Defined in | Defined in (duplicate) |
|---|---|---|
| `BackupType` `server/database/volume/app` | `service.go:47` | `config.go:30` via re-export, `main_service.go` via `BackupType` |
| `DatabaseEngine` 6 constants | `service.go:36` | `artifact.go` + `config.go` |
| `StorageAdapter` | `interfaces.go:86` | `storage.go:15` `StorageBackend` (same method set, different name) |
| `StorageConfig` + `S3/MinIO/Azure/GCS/LocalConfig` | `config.go:46` | `storage_s3.go` uses `S3StorageConfig` etc. defined in same package but `storage.go` imports nothing — configs live only in `config.go` yet `storage_s3.go` defines adapters that consume them |
| `BackupManifest` | `service.go:75` | `artifact.go` also references via `json.RawMessage manifest` + beacon `metadata.go:5` `Backup` has no manifest |
| `RegisterProvider` global map | `service.go:141` | `adapters.go:7` `NewS3Factory` etc. expected to be registered but never called in production |
| `CreateStorageAdapter` | `main_service.go:942` | `adapters.go` factories — two factory styles |

### 5.2 Dead / Unreachable Code

| Location | Why dead |
|---|---|
| `service.go:134` `ProviderFactory` + `RegisterProvider`/`GetProvider`/`RegisteredProviders` | No caller registers factories in production; `adapters.go` defines `NewS3Factory` etc. but none are invoked; `handlers_backup_extended.go:238` `RegisteredProviders()` returns `[]` by default |
| `storage.go:275` `StorageManager` | Never instantiated; `NewStorageManager` not called from any service constructor |
| `storage_s3.go:795` `LocalStorageAdapter` vs `storage.go:44` `LocalStorageBackend` | `LocalStorageAdapter` is the one wired via `CreateStorageAdapter("local",...)`; `LocalStorageBackend` is orphaned |
| `service.go:194` `SetLogger(*log.Logger)` | No-op (`_ = logger`); logger is never used by `Service` beyond `s.log` which uses std `log.Printf` |
| `config.go:176` `if req.CompressionEnabled { req.CompressionEnabled = true }` | Tautology — copies true to true; intended default `true` but `bool` zero-value is `false`, so compression defaults to off despite comment `Default to true` |
| `config.go:184` `if req.Enabled { req.Enabled = true }` | Same tautology — `Enabled` defaults to `false` even though intent is `true`; `Validate` does not enforce enabling |
| `main_service.go:436` `if !req.Enabled { req.Enabled = true }` in `RegisterStorageProvider` | Forces every provider to enabled, ignoring caller intent |
| `restore.go:1177` `determine*RestorePath` trio | Always returns `""` regardless of input; never used except to pass `restorePath` (`""`) to `beaconClient.ExecuteRestore` |
| `restore.go:1154` `verifyDatabaseRestore` / `verifyAppRestore` | Stub — logs and returns `nil`, no verification |
| `encryption.go:32` `GetEncryptionKeyFromEnv` | Reads `FORGE_MASTER_KEY` as 32 bytes, but no service calls it; `Service.UploadBackupWithOptions` takes `UploadOptions.EncryptionKey` directly from caller (caller supplies raw key, never derived) |
| `worker's beacon scheduler duplication` | `beacon/scheduler.go:43` `Scheduler.Schedule` is never called by Forge; gocron instance never started (`StartAsync` missing) |

### 5.3 Duplicated Persistence Records

| Legacy table | New table | Mapping risk |
|---|---|---|
| `backup_policies` (`store_backup_policies.go`) | `backup_configurations` (`store_admin_backups.go`) | `BackupPolicy` vs `BackupConfig` overlap in intent; no migration path; `CleanupOrphanedPolicies` deletes by `applications`/`server_databases` while new `BackupConfig` references `apps`/`databases`/`volumes` stubs |
| `backups` (`store_backups.go`) | `backup_artifacts` + `backup_jobs` (`store_admin_backups.go`, `store_backup_jobs.go`) | `BackupArtifact.StorageProvider/StoragePath` vs `Backup.StorageReceipt`; dual write in `JobService` daemon path creates `BackupArtifact` **and** `Worker` writes `backups` row — no single source of truth |
| `backup_policies.storage` | `backup_configurations.storage_provider` | `s3` vs `S3StorageAdapter` naming divergence (`s3` vs `minio` vs `S3Adapter.Name()=="s3"` for minio) |
| `backup_retention` (`178_*.sql`) | `backup_retention_policies` (`104_a_*.sql`) | Both express retention; neither is queried by the same service |

---

## 6. Provider Abstraction Leaks

### Leak 1 — `StorageAdapter` vs `StorageBackend` dual interface (naming confusion)
`storage.go:15` defines `StorageBackend`; `interfaces.go:86` defines `StorageAdapter` with **identical method set** (`Upload`, `Download`, `Delete`, `List`, `Exists`, `UploadStream`/`DownloadStream`, `GetFileInfo}). They are not type aliases; `LocalStorageBackend` implements `StorageBackend` but **not** `StorageAdapter` (different package-level name), yet `MainService.RegisterStorageAdapter` expects `StorageAdapter`. A local backend built via `storage.go:50` cannot be registered without adapter wrapping.

### Leak 2 — Adapter `Name()` is not the provider key
`S3StorageAdapter.Name()=="s3"` (`storage_s3.go:97`), `MinIOStorageAdapter.Name()=="minio"` (`storage_s3.go:386`), `AzureStorageAdapter.Name()=="azure"`, `GCSStorageAdapter.Name()=="gcs"`, `LocalStorageAdapter.Name()=="local"`. But `MinIOStorageAdapter` wraps `S3StorageAdapter` (`storage_s3.go:355` `type MinIOStorageAdapter struct{ *S3StorageAdapter }`) so `S3StorageAdapter.List` with `prefix` handling differs from `MinIOStorageAdapter` which inherits `S3StorageAdapter.List` but reports name `minio` — storage path prefix logic (`getS3Prefix` vs `blobPath`) diverges per provider yet `CreateStoragePath` (`storage.go:337`) hardcodes `file://` style `backups/<serverID>/<ts>_<name>.tar.gz`.

### Leak 3 — Beacon `AdapterType` (`backup.go:17`) is `local|s3` strings, Forge `StorageAdapter.Name()` is `local|s3|minio|azure|gcs` — `beacon/S3Backup` (`beacon/s3.go:52`) wraps a `LocalBackup` for staging, while Forge's `S3StorageAdapter` does its own `PutObject`/`GetObject`. `ArtifactService.getStorageAdapter` (`artifact.go:901`) falls back to `defaultAdapter` silently (`Warnf` + return default), masking misconfigured provider as success.

### Leak 4 — `StorageConfig` is unmarshaled without validation in `MainService.createStorageAdapter` (`main_service.go:918`): `json.Unmarshal(req.Config, &storageConfig)` succeeds even when `StorageConfig` contains wrong provider's block (e.g., `{"s3":{}}` with `providerType="gcs"`). `CreateStorageAdapter` then checks `config.S3==nil` for `s3` but for `gcs` checks `config.GCS==nil` — if caller sent s3 config with gcs type, error is `GCS config is required` instead of `wrong provider type`.

### Leak 5 — `BeaconClient.ExecuteBackup` (`interfaces.go:49`) takes `StorageAdapter` as argument — the API node receives a server-side adapter object (with credentials) over the wire? In practice `job.go:575` passes `storageAdapter` (which holds `aws.Config` with secret keys) to `beaconClient.ExecuteBackup(ctx, nodeID, type, targetID, name, storageAdapter)` — serializing cloud credentials to the beacon payload. The direct `daemonClient.CreateBackup` path avoids this but still sends no credentials — daemon must have its own storage config, creating split-brain.

---

## 7. Queue Coupling Bugs

### Q1 — No queue; synchronous job execution inside request / tick (see L03)

### Q2 — Beacon task polling has two divergent implementations

| Location | Interval | Timeout | Backoff |
|---|---|---|---|
| `job.go:975` `waitForBeaconTaskCompletion` | `time.Second` initial → `backoff*=2` capped 30s | 30 min | in-job.go only |
| `restore.go:1262` `waitForBeaconTaskCompletion` | `2 * time.Second` fixed | 30 min (`deadline = now+30m`) | none |

If beacon task runs >30 min (large DB), job path times out and returns error, which `Execute` then marks as `failed` with retry; restore path does the same but with different polling cadence — observable jitter difference can cause one path to succeed while the other fails on the same task.

### Q3 — `Worker.Stop` race (`worker.go:39`)

```go
select { case <-w.stopCh: default: close(w.stopCh) }
```
`Stop` holds `w.mu.Lock`, but `loop` reads `w.stopCh` via `select { case <-w.stopCh: return }` without lock. If `Stop` is called concurrently with `Start` (`worker.go:61` `w.stopCh = make(chan struct{})` under `mu`), `loop` may be reading the old `stopCh` that `Stop` just closed, while `Start` replaces it — lost wakeup. `Start` is not idempotent under concurrent calls.

### Q4 — `ListRetryableBackupJobs` includes `failed` jobs with `retry_count < max_retries` even when `last_retry_at` is null (first failure) — every tick picks them up immediately, causing tight 1-min retry loop for 5 jobs serially, saturating beacon. `withRetry` already retries per-call 3×; DB backoff `LEAST(1<<retry_count,30)` minutes means 1st retry after 2 min, but `pickupBackupJobs` runs every 1 min — job sits idle for 1 tick, then retries. No jitter on DB backoff (only `withRetry` has `jitterDuration`).

---

## 8. API Without Runtime

| API | Declared | Runtime |
|---|---|---|
| `POST /admin/backups/configs/:id/execute` (`handlers_backup_extended.go:310`) | creates job + executes inline | works (sync) but blocks request; no queue drain if API restarts mid-execute |
| `POST /admin/backups/jobs` (`handlers_backup_extended.go:331`) | create + execute inline | same |
| `GET /admin/backups/configs|s` `POST .../configs` `PATCH` `DELETE` | CRUD via `ConfigService` | works (store only), but `ListBackupConfigs` loads all rows into memory (`config.go:250` `ListBackupConfigurations` no LIMIT) — paginates in Go after full scan; with 10k configs OOM risk (mirrors Kopia's manifest load but without index) |
| `GET /admin/backups/jobs` `DELETE /admin/backups/jobs/:id` | via `JobService.List` (SQL paginated) vs `DeleteBackupJob(status NOT IN running)` | `DELETE` is soft-block on running but API returns 409; no force-cancel path |
| `GET /admin/backups/artifacts` `DELETE /admin/backups/artifacts/:id` `POST .../lock|unlock` `GET .../download` | via `ArtifactService` | `download` (`handlers_backup_extended.go:422`) loads entire artifact into `[]byte` via `adapter.Download` then wraps in `bytesReader` (`artifact.go:612` `newBytesReader`) — no streaming, OOM on large archives; Restic/Kopia stream via `repo/open` |
| `GET /admin/backups/storage-providers` `POST /storage-providers` `SetDefault` | `MainService.RegisterStorageProvider` + `ListBackupStorageProviders` | `RegisterStorageProvider` (`main_service.go:417`) writes `adapter.Upload`-capable credentials as raw `json.RawMessage` encrypted via `secretAAD`; `List` redacts config (`includeConfig=false`) but `GetStorageProvider` returns `storageProviderFromRecord(record,false)` — always redacted, so edit flow cannot read back config to populate form |
| `POST /admin/backups/restore` `GET /admin/backups/restores` | `RestoreService.Create` + `Execute` inline | `Execute` downloads artifact to local temp via `DownloadToFile` (`artifact.go:616`) then dispatches to beacon — double write (temp dir + staging dir in beacon) |
| `GET /admin/backups/status` (`handlers_backup_extended.go:495`) | aggregates 4 lists per request (configs/jobs/artifacts/restores) each `PerPage:200` then counts in handler | N+1 fan-out on every page load; `backupStatusResponse` re-implements `GetBackupSystemStatus` (`main_service.go:773`) logic — divergent counts if one path fixes a bug and the other doesn't |
| `POST /servers/:id/backups/*` (`handlers_servers.go`) | legacy per-server file endpoints (`download`, `restore-status`) | talks to `Service` + `daemon` directly, bypasses `backup_jobs`/`backup_artifacts` tables — creates orphan `backups` rows without corresponding artifacts |
| `GET /backup/providers` (`handlers_backup_extended.go:237`) | `RegisteredProviders()` global map | always empty after migration to `backup_storage_providers` table |

**Coverage gap:** Volume-aware flows (`BackupTypeVolume` requires `server_id+volume_id`, `config.go:521`) have no HTTP route — only database and server flows are exposed via `handlers_servers.go` / `handlers_database_services.go`. A volume backup created via `MainService.CreateBackupConfig` (via admin API) cannot be listed via per-server endpoints and its artifact is only reachable via `/admin/backups/artifacts` (admin-only).

---

## 9. Beacon Adapter Deep-Dive vs FORGE Adapters

| Dimension | Beacon `LocalBackup` (`beacon/local.go`) | Forge `LocalStorageAdapter` (`storage_s3.go:797`) |
|---|---|---|
| Archive format | `.zip` (zip/Deflate) | `.tar.gz` (tar + gzip/zstd) — mismatched with beacon validation |
| Name validation | `validBackupName` requires `.zip`, no slash, alphanumeric+`-_` | no validation beyond `securePath` in sibling backend (not used) |
| Namespace | `validNamespace` (server UUID, `backupRoot/<ns>`) | `storagePath = backups/<serverID>/<ts>_<name>.tar.gz` (timestamp-prefixed) — not compatible |
| Integrity | `.metadata.json` sidecar + `calculateChecksum` post-write + validation on every `Get`/`Restore`/`Download` | `ChecksumVerified` bool on `backups` row, `VerifyChecksumFromStore` re-downloads full blob |
| Concurrency | `namespaceMu` + `namespaceOperation{mu, refs}` per-namespace serialisation | `sync.RWMutex` on `StorageManager` / no lock on `LocalStorageAdapter` |
| Journal | crash-consistent swap with `restoreJournal` | no journal in Forge layer — relies on beacon journal when using daemon, none otherwise |
| Disk guard | `S3Backup.ensureStagingDiskSpace` (`beacon/diskspace.go:14`) with 64 MiB reserve | none — Forge `S3StorageAdapter.UploadStream` `io.ReadAll(reader)` unbounded |
| S3 hardening | `validateS3Endpoint` enforces HTTPS except loopback (`beacon/s3.go:100`), 50 GiB download cap, SHA-256 metadata required | `S3StorageAdapter` has no endpoint validation beyond trim, `GCSStorageAdapter` parses JWT per-request without caching |

---

## 10. Migration Retention Deep-Dive

| Migration | Table | Retention mechanism |
|---|---|---|
| `019_backups.sql` | `backups` | no retention |
| `049_backup_locking.sql` / `051_backup_status_tracking.sql` | adds `is_locked`, `status` | lock prevents delete |
| `132_backup_schedules_orchestration.sql` | adds `backup_policies.app_id/service_id/database_type`, `backup_manifests`, `backup_storage_receipts` | `cleanup_orphan_backup_policies` trigger (never attached) |
| `162_backup_policy_next_run_and_instance_attempts.sql` | adds `backup_policies.next_run_at` | consumed by `Worker.persistNextRun` |
| `178_backup_retention.sql` | `backup_retention(kind, retention_days, retention_max)` | dedicated retention for `managed_database_backups` — not connected to `backup_policies` or `backup_retention_policies` |
| `104_a_backup_system.sql` | `backup_retention_policies(scope, max_backups, retention_days/weeks/months)` | `ArtifactService.ApplyRetentionPolicy` consumes it; no trigger, manual `ApplyRetentionPolicy` call only |

**Gap:** Three retention tables, zero unified Gc loop. `backup_retention` is engine-keyed but `managed_database_backups` never references it from code path `store_backups.go`; `backup_policies` is server-scoped; `backup_retention_policies` is the only multi-scope table but is only invoked via explicit `POST /retentionPolicies/:id/apply` (`main_service.go:759`).

---

## 11. Recommended Remediation (Prioritized)

1. **Merge `Service` into `MainService` or delete `Service`.** Remove `service.go`'s global `providerFactories`, `StorageManager`, `EnforceRetentionPolicy`; port `Worker` to drive `ConfigService`/`JobService` via a single `BackupService` interface. Single adapter registry with `StorageAdapter` only; delete `LocalStorageBackend` or make `LocalStorageAdapter` delegate to `securePath`.

2. **Adopt single retention engine.** Delete `store_backups.go:CleanupOldBackups`, `backup_retention` table, and `Worker.enforceRetentionBeforeBackup` pre-delete. Keep `ArtifactService.ApplyRetentionPolicy` as the sole enforcer, driven by a dedicated retention worker that scans `backup_retention_policies` ordered by `priority` and applies `scope` correctly. Add `ORDER BY CreatedAt DESC` to `ArtifactService.List` before applying `index`-based `MaxBackups`.

3. **Single `storagePath` helper.** Extract `func canonicalStoragePath(name string) string` that normalizes suffix via `HasSuffix` and is used by `Upload`, `Download`, `Delete`, `GetBackupByName`, `VerifyStorageReceipt`. Backfill existing rows; add DB constraint on `storage_path`.

4. **Introduce async queue.** Replace synchronous `ExecuteBackupJob` in handlers with `Enqueue(jobID)` → 202. Worker pulls from `backup_jobs WHERE status='pending' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1` (or use `pgboss`). Reap `running` older than `30m` to `failed`.

5. **Streaming crypto/compression.** Replace `EncryptReader` double-buffer with `cipher.AEAD.Seal` streaming or `chunked` encryption; persist `compression_algorithm` per artifact; don't rely on env at read time.

6. **Wire schedulers.** Remove `beacon/scheduler.go` duplication (or make it the only scheduler and have Forge push `BackupConfig` cron entries to it). Make `MainService.SetScheduler` mandatory and run a retained tick that lists `backup_configurations WHERE is_scheduled AND next_run_at <= now()`.

7. **Fix local adapter path traversal.** Make `LocalStorageAdapter.Upload` call `securePath` (`storage.go:244`) before any `filepath.Join`.

---

## 12. References

- Forge: `forge/api/internal/services/backup/{service,main_service,config,job,artifact,restore,storage,storage_s3,adapters,interfaces,worker,compression,encryption,retry}.go` (`service.go:1`, `main_service.go:1`, `storage.go:1`, `storage_s3.go:1`, `worker.go:1`)
- Beacon: `beacon/internal/backup/{backup,local,s3,retention,scheduler,verification,store,diskspace}.go` (`backup.go:17`, `local.go:40`, `s3.go:69`, `retention.go:35`)
- HTTP: `forge/api/internal/http/{handlers_backup_extended,handlers_servers,handlers_apphosting,handlers_db_containers}.go` (`handlers_backup_extended.go:35`)
- Migrations: `forge/api/migrations/{019_backups,049_*,051_*,104_a_backup_system,132_backup_schedules_orchestration,178_backup_retention}.sql`
- Store: `forge/api/internal/store/{store_backups,store_backup_policies,store_backup_jobs,store_admin_backups,store_backups_admin}.go`
- Reference: `reference/backup/kopia/repo/content/*`, `reference/backup/restic/internal/repository/pack/*` (content-addressable, rolling hash, snapshot manifests) — contrast with Forge's full-archive `BackupArtifact` (`artifact.go:21`)

---

*Audit performed by Subagent 04 — Phase 03. No code was modified; findings are evidence-backed via file:line references. Verification via `rg` + file reads; no runtime execution.*
