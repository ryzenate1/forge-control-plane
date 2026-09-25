# Phase 03 Synthesis — Backup Cluster (Kopia + Restic)

**Cluster:** kopia (master), restic (master)  
**Why this cluster now:** Phase 1 flagged `operation vs queue` worker divergence and Phase 2 `restoring_backup` lock missing; both assumed backup/restore was “just a job.” Backup is the subsystem where correctness (dedup vs artifact, integrity vs sidecar, retention AND vs OR) and operability (progress, verify, prune, disaster recovery) decide data-loss risk. Kopia (content-addressed blobs+index+maintenance) and Restic (pack files+index+locks+prune) solve the same encrypted deduplicated repository with opposite designs — together they test every Forge backup seam.  
**Evidence:** 5 subagent reports (14–18 comparisons each) — model, crypto/retention, restore/resumability, Forge arch accretion, backup ops UX — file:line verified.  
**Forge surface:** `forge/api/internal/services/backup/*` (`artifact.go` `main_service.go` `service.go` `storage.go` `storage_s3.go` `encryption.go` `job.go` `restore.go` `config.go` `compression.go`), `beacon/internal/backup/{backup,local,s3,store,retention,scheduler,verification}.go`, `forge/api/store_backup*.go`, `forge/api/migrations/*backup*`, handlers `handlers_backup*`, `forge/web` backups views (`AdminBackups`, `backups-view.tsx`), `server.go:backupProgressWS`.

---

## 1. Reference capabilities (what good looks like)

| Concern | Kopia | Restic |
|---------|-------|--------|
| Repo model | Content-addressed blobs (`repo/blob/storage.go:204` implements `BlobStorage`), `content_manager.go:70` content IDs, `manifest_manager.go:72`, chunked content-index, deduplication by rolling hash; sparse index + manifest | Pack files (`internal/repository/repository.go:32` `Repository` with `PackFile`, `Chunker` `chunker.go:9` Rabin fingerprint), index `index.go` (`LoadIndex`/`LookupBlobSize`), snapshots as trees (`internal/restic snapshot.go`), pack+index compaction via `prune.go:106/386`, exclusive locks `lock.go:47/124` |
| Integrity | Per-blob AES-GCM+HKDF, AEAD MAC per write, `command_snapshot_verify.go:18` `snapshot verify --verify-files-percent`, `command_content_verify.go` | Poly1305-AES 16B nonce, `cmd_check.go`/`cmd_prune.go` repo `check --read-data`, `restorer/verifyFile` per blob `Hash`, `index` reconstruction |
| Retention/pruning | `retention.go:61` policy union (keep latest N, hourly/daily/weekly, `KeepWithin`), `maintenance.go` GC roots; `ActionsPolicy` hooks `BeforeFolder/AfterFolder` etc. hooks `actions_policy.go` with `Command/Script/Timeout` | `cmd_forget.go`/`cmd_prune.go` `--keep-*` union OR, `prune` inside exclusive lock + `rebuild-index`; scheduler via system `cron` or `--schedule`; `forget --prune` atomic |
| Restore | Shallow/parallel `snapshot/restore` queue, glob `**` / path filtering, checkpoint resumes, progress `ProgressCallback` throttled 1/s | `internal/restorer` `SelectFilter` subtree pruning, `AppendToStdout` partial, progress via `ui/restore.Progress`, `fileState` blob resume |
| Operations UX | `kopia snapshot create --json`, adaptive ETA counters (hashed/estimated/failed), maintenance counters, `kopia ui` 5 counters | `restic backup --json` `message_type: status` (`ui/backup/json.go:42`) every 0.2s with `bytes_done/total_bytes`, `restic --json` streaming, `restic snapshots --json`, `check --read-data` status |

---

## 2. Forge equivalents (map)

| Concern | Forge location | Pattern |
|---------|----------------|---------|
| Artifact model | `services/backup/artifact.go` `BackupArtifactRecord` + `storage.go:14` `StorageAdapter` | Per-server artifact `backup.zip` + sidecar `backup.zip.metadata.json` {uuid,size,checksum,time} (`beacon/local.go:787`); listing via `os.ReadDir` readdir not index |
| Encryption | `services/backup/encryption.go:92-124` `EncryptReader/DecryptReader` `AES-256-GCM`, `keyring.go:75` AEAD; `beacon/encryption.go` mirroring | Single-archive envelope, **not** per-blob; `local.go:787` sidecar SHA-256 hex unauthenticated |
| Storage | `storage.go:14` `StorageAdapter` + `storage_s3.go:126` `S3Adapter` `manager.NewUploader` 3-attempt backoff; `local.go:238/468` local zip create | Two local backends (secure `local.go` vs insecure per concerns in subagent04), S3 single prefix per server (`storage_s3.go`) |
| Scheduling | Beacon `gocron` `scheduler.go`, API `Worker` `policyDue(NextRunAt)` `persistNextRun(cron.ParseStandard)`, `MainService`/`ArtifactService:522` duplicate `VerifyAllBackups`/`ArtifactScheduling` | Three schedulers: beacon `gocron`, API `Worker` tick, `services/backup` internal ticker |
| Restore | `services/backup/restore.go` `RestoreOptions{StopAppBeforeRestore,StartAppAfterRestore}`, `beacon/local.go:111` `restoreJournal` 3-phase `prepared→live-moved→activated` + `RecoverRestoreJournals`, `.partial` `O_EXCL`, `downloadToStaging .s3-download-*.zip`, `calculateChecksum` SHA256 | Journaled 3-phase rename (`MkdirTemp("."+base+".restore-")` → journal → `Rename(live,rollback)` → `Rename(staging,canonical)`) with `syncDirectory` |
| Progress/WS | `beacon/backup/backup.go:1` `BackupProgress{BytesProcessed,TotalBytes,Phase}` + `server.go:1542` `eventBus BackupProgressEvent` + `backupProgressWS:2085` via `eventBus`, `forge/api/http/server.go:1975` realtimeProxy `console\|stats\|logs` only | Beacon publishes but **panel never proxies** (hardcoded union `lib/api.ts:933`), web polls `GET /backups` 3s (`backups-view.tsx:43`) with `TotalBytes=0` until completion |
| Retention | Three engines: `beacon/retention.go:61` AND, `store_backup_policies.go:301` hard-coded 30d, `MainService:config.go:170-187` defaults vs `ArtifactService:536` intersection | AND vs OR inversion vs Restic/Kopia union OR |
| Verification | `beacon/verification.go`, `ArtifactService.Verify` (not scheduled) | Unscheduled, not streamed |

---

## 3. Hidden / unwired

- `backupProgressWS:2085` exists and beacon fires `SetProgressCallback→eventBus` every `writeLimit` chunk, but `forge/api/http/server.go:1975` only proxies `console|stats|logs`; `forge/web/lib/api.ts:933` hardcoded union drops `backup` — progress appears dead despite plumbing (**major unwired**, subagent05).
- `VerifyAllBackups` (`ArtifactService:522`) / `ArtifactScheduling` never scheduled; `storage_s3.go` S3 prefix discoverability never surfaced in admin UI (`AdminBackups` lists jobs not artifacts).
- `pre-restore-*.zip` snapshot `server.go:1722` hardcoded journal name `rollbackName` visible only via server filesystem, no `restoreJournal` recovery UI.
- Beacon `local.go:90` `lockNamespace` in-process only — no cross-replica exclusion (Restic `lock.go:47` exclusive repo lock vs Forge `lockNamespace` local dict).

---

## 4. Missing (genuine)

- Chunked dedup/content-addressed store (Kopia/Restic) vs Forge whole-archive zip — gap only matters for >1GB worlds where incremental wins; for typical Minecraft-scale (<2GB) artifact model suffices. If targeting large modded/Rust worlds, need chunked adapter (Kopia-as-adapter toggle) not new system.
- Per-blob AEAD + HKDF salt per write vs Forge single-archive `nil` salt envelope — missing for multi-tenant cross-server isolation (forge `encryption.go:26` reuses same `keyID` with `nil` salt).
- Bandwidth token-bucket per op (Kopia) vs Forge single `rate.Limiter` only on zip-create write path.
- Inter-entry conditional hooks (`ActionsPolicy` 4 hook points with `KOPIA_SNAPSHOT_PATH` capture) vs Forge hardcoded pre-restore snapshot only.

---

## 5. Broken / incorrect logic (FORGE LOGIC FINDINGS — 16 primary)

### Crypto / integrity (P0/P1)

- **F-B-01 — Unauthenticated sidecar + null AAD → cross-server transplant forgery.** `beacon/local.go:787-817` writes sidecar `{"checksum":sha256}` and `local.go:757` `readOrCreateMetadata` trusts it without rehash; `services/backup/encryption.go:26-32` `EncryptReader` uses `nil` salt/AAD when `keyID` present. Attacker can transplant `backup.zip`+metadata from server A to B. (subagent02 L1 HIGH)
- **F-B-02 — `EncryptReader` buffers entire backup → heap OOM.** `encryption.go:150-204` `io.ReadAll` before GCM seal; 20 GiB world → OOM even with streaming S3 uploader (`storage_s3.go` streams but input already buffered). Kopia/Restic stream per chunk. (L2 HIGH)
- **F-B-03 — Unthrottled progress fan-out / future per-file progress blocked.** `local.go:111` `reportProgress` per `writeLimit` (default 1M) fires WS per MB without `Throttle.ShouldOutput` guard; also blocks planned per-file progress (`beacon/local.go:111→server.go:1542→backupProgressWS:2085`). (subagent03 LF-03 P2)
- **F-B-04 — Sidecar trusts stale checksum without rehash.** `local.go:757-785` `readOrCreateMetadata` returns `metadata.Checksum` if sidecar exists, else computes — tamper bypass. (subagent03 LF-02 P1)

### Retention / GC (P1 — data-loss risk)

- **F-B-05 — Retention AND vs OR inversion → silent loss/bypass.** `retention.go:61` `shouldKeep` is AND (must pass latest+hourly+daily+weekly), while `store_backup_policies.go:301` hard-coded `30d` and `MainService config.go:170-187` defaults diverge; Restic/Kopia are union OR (keep if any rule keeps). Over-retention delete or under-retention keep depending which engine wins. Three engines disagree. (L3 HIGH)
- **F-B-06 — SQL-only prune orphans S3/GCS objects + racy `enforceRetentionBeforeBackup`.** `store_backups.go:242-301` `DeleteExpired` deletes rows without S3 `RemoveObject`; `services/backup/main_service.go:406-444` `enforceRetentionBeforeBackup` loops `enforceRetention` then `Delete` without Tx, double-delete on retry. Restic prune holds exclusive `lock.go:47`. (L4 HIGH)
- **F-B-07 — Staging OOM + GC orphan/no-index.** `local.go:468` `createStagingDir` writes under `serverRoot/.metadata-*` with `.partial` `O_EXCL` but GC (`scheduler.go`+`store.go` `cleanupExpired`) only sweeps DB `backup_jobs` not `.partial`/`.restore-*`/`.s3-download-*`; `store_backups.go:242` prune is sweep but not index-backed, so `.partial` left after crash never indexed. Truncated in subagent01 logic finding. (subagent01 L1/L2)

### Concurrency / staging / journal

- **F-B-08 — Tandem `lockNamespace` not distributed.** `beacon/local.go:90-109` `lockNamespace` is `map[string]bool` in-process; Beacon replica count >1 can `Create` same namespace concurrently; Restic `lock.go:124` `LockExclusive` is repo file (distributed). (subagent01 L3, subagent02 #12)
- **F-B-09 — `snapshotErr != nil` skips rollback discovery.** `beacon/server.go:1720-1744` `restoreBackup` on `Create(rollbackName)` error returns 500 without cleaning stale `pre-restore-*.zip` sidecar with bad checksum — leaks and blocks next restore’s `list` (`bad metadata`).
- **F-B-10 — Retry accumulates `pre-restore-*` artifacts.** `restore.go:546-577` `Retry` re-enters `Execute` sync accumulating temp dirs without cleanup. (subagent03 LF-04 P2)
- **F-B-11 — Path-suffix mismatch upload vs download.** Upload `HasSuffix(name, ".zip")` vs download `strings.Contains(name, ".")` → `backup` vs `backup.zip` accepted upload but rejected download (subagent04 C path).
- **F-B-12 — Synchronous execution inside HTTP/tick, claimed never reaped.** `BackupService:110/201` marks `running` but worker `201` not reaped like `queue` (subagent04 C15 sync).
- **F-B-13 — Four retention tables with AND/OR/intersection divergence** (`retention.go` AND vs `store_backup_policies` 30d vs `ArtifactService:536` intersection vs Restic prune lock) — correctness divergence not just duplication.
- **F-B-14 — Adapter registries triple-registered** — `storage.go` adapter map vs `MainService` adapter field vs proposed `Provider` registry `adapters.go` pattern not adopted.

### Operations UX (P2 — dead progress)

- **F-B-15 — Backup progress WS pipeline dead end.** `beacon/server.go:1542` publishes `BackupProgressEvent` but `forge/api/http/server.go:1975` only proxies `console|stats|logs` and `forge/web/lib/api.ts:933` `lib/api/servers.ts:unknown` drops `backup` union — 3s poll shows `TotalBytes=0` until completion (`local.go:111`). Adaptive ETA impossible. (subagent05 primary)
- **F-B-16 — Full-download verify timeout.** Backup verify downloads whole S3 object to compute SHA256 (`verification.go`), not streaming nor range — large artifact timeout vs `restic check --read-data` incremental.
- **F-B-17 — Pending zombie after page unload.** No cancellation token on `CreateBackup` job; `job.go` `running` persists after client disconnect — orphan.
- **F-B-18 — Storage/retention saves are no-ops hidden behind 200.** Admin saves to `backup_configurations` with `provider=s3` not wired to `storage_s3.go` discovery; read returns stale.

---

## 6. Duplicates

- `services/backup/service.go:271` (legacy monolith) vs `main_service.go` `MainService{Config,Job,Artifact,Restore}Service` quartet — both expose `CreateBackup` with divergent validation; legacy still called by `queue.Worker` path. Both write `backups` rows via different stores.
- `local.go` secure staged archive vs direct `writeFile` insecure path coexist; migration `051_backup_status_tracking` added `status` but `local.go` status `pending` string distinct from `ArtifactService` enum.
- Three schedulers enqueued same retention cycle (`beacon gocron` + API `Worker` `policyDue` + `MainService` internal ticker).
- Four retention/verify paths (`retention.go` AND, `store_backup_policies` hard-coded, `MainService` defaults, `ArtifactService` intersection) as above.

---

## 7. False completion / decorative

- `GET /backups` lists `backups` rows even when S3 object already evicted via manual console delete — row exists but download 404.
- `storage_s3.go` prefix discovery returns 200 empty list when `BACKUP_S3_BUCKET` unconfigured — page renders empty not “S3 not configured” warning.
- Verify button fires `verification.go` verify then `200 {"verified":true}` even when running — async verify result not polled.

---

## 8. Architecture lessons

| Reference pattern | Forge today | ADOPT / INSPIRE / REJECT |
|-------------------|-------------|---------------------------|
| Content-addressed index + GC roots (Kopia manifest manager / Restic index+prune) | Whole-archive zip + readdir list | **INSPIRE for >5GB case** — keep artifact model for typical game saves, but add chunked adapter toggle (Kopia as adapter via `adapters.go`) rather than rewriting Forge to pack/index |
| Per-blob HKDF/AEAD with per-write nonce | Single-archive `nil` salt GCM + sidecar SHA | **ADOPT** per-write salt/nonce + AAD binding `server_id:backup_name` in `encryption.go` |
| Repo exclusive lock file (`restic lock.go:47`) | In-process `lockNamespace` dict | **ADOPT** distributed lock (DB row `advisory_lock` or S3 conditional write) for multi-replica beacons |
| Union retention OR (keep if any rule) + `prune` inside lock | AND retention + SQL delete | **ADOPT** union OR and make prune `SELECT … FOR UPDATE` + S3 delete in same Tx or mark sweep |
| Streaming encryption per chunk | `io.ReadAll` then seal + sidecar | **ADOPT** `cipher.StreamWriter` chunked seal (Kopia `encryptChunk`) |
| Progress throttled push (`Throttle.ShouldOutput` 1/s) + `message_type: status` JSON cadence | Unthrottled `reportProgress` + `TotalBytes=0` until completion | **ADOPT** throttled callback + `bytes_done/total_bytes` estimate from staging stat |
| Hooks at 4 lifecycle points with SCRIPT/COMMAND/Timeout/Mode | Hard-coded pre-restore snapshot | **INSPIRE** — add minimal pre/post hooks (stop game → snapshot) via existing `RestoreOptions.StopAppBeforeRestore` expansion, not full Kopia `ActionsPolicy` |

---

## 9. Recommended activation order (existing capability → wired)

Without new subsystem (all within existing `backup/*`, `beacon/*`, `store_*`):

1. **P0 Wire `backupProgressWS` end-to-end** — add `backup` to `server.go:1975` proxy union + `lib/api.ts:933` union + web `BackupsView` consume WS; set `TotalBytes` via `Stat` of staging `.zip` early so adaptive progress meaningful (F-B-15). Unblocks “backup appears dead”.
2. **P0 Fix encryption streaming + salt** `encryption.go:150` replace `ReadAll` with `StreamWriter`, salt per backup `server_id:uuid`, bind AAD `server_id:backup_name` (F-B-02/01/03).
3. **P0 Unify retention to union OR** — converge `retention.go:61`, `store_backup_policies.go:301`, `MainService` defaults to single `KeepWithin/KeepHourly…` union OR like Restic/Kopia; add Tx holding `SELECT … FOR UPDATE` + S3 sweep mark, delete orphan DB rows + `.partial` GC (F-B-05/06/07).
4. **P1 Make prune exclusive** — add DB advisory lock `backup_prune` around `store_backups.go:242` delete + S3 `RemoveObject` loop; or mark sweep (`status=pruned_pending`) then GC worker (like Restic `lock.go:47`).
5. **P1 Fix restore staging journal** — clean stale `pre-restore-*.zip` on `snapshotErr`, persist `RestoreOptions` in journal `readOrCreateMetadata` rehash on sidecar tamper, rollback via `RecoverRestoreJournals` on startup (F-B-09/10/04).
6. **P1 Deduplicate backup services** — deprecate `service.go` monolith, route all via `MainService` quartet; reuse `StorageAdapter` registry (`adapters.go`) for `local|s3` (and future kopia/restic adapter), single scheduler `Worker` (deprecate beacon `gocron` duplicate).
7. **P2 Add per-file progress & hooks** — throttle `reportProgress` 1/s (`Throttle.ShouldOutput`), expose minimal pre/post hooks (stopApp → backup → startApp) in `job.go` already partially present via `RestoreOptions`.
8. **P2 Add full-download verify streaming** — stream `S3 GetObject` range reads not full download; expose `GET /backups/:id/verify` async job with poll.
9. **P3 Fix path-suffix + TotalBytes early** — normalize `backup` ↔ `backup.zip` in upload/download, stat `size` early for `TotalBytes` estimate.
10. **P3 Wire disaster recovery** — document `onboarding_tokens` → `recovery` flow for backup seed, surface `Restore after onboard` in `forge install` wizard.

---

## 10. Handoff to Phase 04 (Networking)

Networking (Caddy/Traefik/NPM vs `domains/traffic/loadbalancer/servicediscovery/crossnode/dns/acme`) reuses backup’s `storage_s3` S3 pattern for cert persistence (already seen). Phase 4 should not re-inspect whole-archive vs chunked model and should instead focus on cert validation, dynamic routing, and service discovery mesh that Phase 3 did not go deep on.

