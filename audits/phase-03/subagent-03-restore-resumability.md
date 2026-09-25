# Subagent 03 — RESTORE / PARTIAL RESTORE / RESUMABILITY / BANDWIDTH / SCHEDULING / HOOKS

**Dimension:** restore (full vs partial, path filtering), resumability, bandwidth controls, scheduling (cron), hooks, catalog/index, resumable upload/download, progress  
**Cluster:** kopia, restic  
**Date:** 2026-08-23  
**Scope:** `reference/backup/kopia/snapshot/restore/*`, `reference/backup/restic/internal/restorer/*`, `reference/backup/restic/cmd/restic/cmd_restore.go`, `reference/backup/kopia/snapshot/policy/*`, `reference/backup/kopia/repo/blob/throttling/*`, `forge/api/internal/services/backup/restore.go`, `forge/api/internal/services/backup/job.go`, `forge/api/internal/services/backup/worker.go`, `forge/api/internal/services/backup/service.go`, `beacon/internal/backup/*`, `beacon/internal/transfer/protocol.go`, `beacon/internal/transfer/transfer.go`, `beacon/internal/server/server.go` (backupProgressWS, restoreBackup, createBackup), `forge/web/components/server/backups-view.tsx`

---

## 1. Comparisons (14)

### C-01 — Full vs partial restore (truncate / targeted paths)

| Reference | Forge |
|-----------|-------|
| **Kopia** `snapshot/restore/restore.go:98-111` `Options{Incremental, DeleteExtra, IgnoreErrors, Parallel, RestoreDirEntryAtDepth, MinSizeForPlaceholder}` drives `copier` — full restore is `MaxDepth=0`; shallow restore via `shallowoutput.WriteDirEntry` at `currentdepth > maxdepth` (`copyDirectory:274`). Parallel queue (`parallelwork.Queue`, workers = NumCPU or 1 if not `Parallelizable()`:139-145). <br>**Restic** `restorer/restorer.go:345-485` `RestoreTo(dst)` always full tree walk but `SelectFilter func(item string, isDir bool)(selected, childMayBeSelected bool)` (`restorer.go:31`) implements include/exclude and `snapshotID:subfolder` via `FindTreeDirectory` (`cmd_restore.go:165`). | **Beacon** `beacon/internal/backup/local.go:468-542` `Restore(ctx, namespace, name, serverRoot string, truncate bool, paths []string)` — two modes: `truncate=false` → direct `extractArchive` via `rootfs.FS` (`537-541`); `truncate=true` → staging swap (`MkdirTemp(parent, "."+base+".restore-"):546` + `extractArchive` into staging + 3-phase journal swap: `prepared→live-moved→activated`:574-622). Targeted restore forces `truncate=false` (`528-529`) and `selectRestoreEntries` filters to requested paths + ancestors (`731-755`), rejecting absolute/`..`/`\`. <br>**Forge API** `beacon/internal/server/server.go:1691-1779` `restoreBackup` always creates `pre-restore-*.zip` snapshot (`1721-1722`) serialized under `backupMu` (`1721/1739`) and auto-rollback on failure (`1732-1737`). Control-plane `forge/api/internal/services/backup/restore.go:779-1152` has `executeAppRestore/executeVolumeRestore/executeDatabaseRestore/executeServerRestore` each with phases `validating→creating pre-restore snapshot→downloading backup→journaling restore intent→activating restore`. |

**Gap:** Forge's partial restore is path-prefix filtering on ZIP entries only; there is no Kopia-style placeholder-at-depth or Restic-style subtree-rooted tree walk (which avoids scanning unselected subtrees via `childMayBeSelected`). Beacon's `selectRestoreEntries:748` scans *all* `reader.File` and includes ancestor dirs for any match (`name==p || prefix || prefix-reverse`), which is correct for ZIP flat lists but O(N*M) and has no parallel extraction.

---

### C-02 — Path filtering semantics

| Reference | Forge |
|-----------|-------|
| **Restic** `cmd_restore.go:195-238` `selectExcludeFilter` / `selectIncludeFilter` built from `ExcludePatternOptions` / `IncludePatternOptions` via `filter.RejectByPattern`/`IncludeByPattern` (glob + `**`, mutual exclusion enforced:134). Two-return `(selectedForRestore, childMayBeSelected)` enables pruning entire subtrees without descending. <br>**Kopia** filtering is via `fs` iteration; delete-extra path `deleteExtraFilesInDir:309-378` reads live dir vs snapshot `GetAllEntries`, classifies dirs vs files, handles case-insensitive FS (`toComparableFilename`). | **Beacon** `local.go:731-755` `selectRestoreEntries` validates each `paths` entry is not `""`, no `\`/`\x00`, `path.Clean != "."/".."` nor `"/"`/`"../"` prefix, then matches `name == p || strings.HasPrefix(name, p+"/") || strings.HasPrefix(p, name+"/")` — ancestor inclusion ensures parent dirs exist. Error if `len(selected)==0` (`526`). |

**Gap:** No include/exclude glob (`**`, `fnmatch`) in Forge; only exact prefix match. Kopia/Restic both support glob patterns; Forge `ignored` applies only at backup creation (`local.go:298-322` denylist) not restore filtering.

---

### C-03 — Resumability (backup creation & restore swap)

| Reference | Forge |
|-----------|-------|
| **Kopia** resumable **snapshot upload** via `repo/content/content_manager.go` checkpoint + `snapshot/upload/upload.go` content-manager flush with `checkpoint` and `FlushResumesWriters` test (`content_manager_test.go:1100`). Server-level `resume` / `resume-source` control API (`internal/server/api_snapshots.go:208`, `source_manager.go:289`, `cli/command_server_resume.go:13`) unpauses paused sources; not per-file upload resume. <br>**Restic** no user-visible backup resume; **restore** is two-pass and stateless (`RestoreTo` first pass `traverseTree` create dirs + `filerestorer.addFile`, second pass `restoreNodeTo` metadata; `verifyFile:714` supports `trustMtime` fast path but not offset resume). Resume is at blob deduplication level: `fileState.blobMatches` marks already-correct blobs to skip (`verifyFile:742-772`). | **Beacon backup creation** `local.go:275-393` resumability via **temp-file atomicity**, not offset: writes to `backupPath+".partial"` with `O_CREATE|O_EXCL` (`275`), defers `os.Remove(partial)` if not committed (`283-285`), `temp.Sync()` then `os.Rename(partial, backupPath)` (`390-393`). On context cancel (`copyWithContext` → `contextReader.Read` checks `ctx.Err()`), archive is deleted; retry requires full re-upload (idempotency via `Get` before `Create` in `server.go:1547`). <br>**Beacon restore** `local.go:544-624` swap is **crash-resumable** via `restoreJournal` (`33-38`) written at each phase (`writeRestoreJournal:899`), recovered at boot by `RecoverRestoreJournals(serverDataRoot)` (`867-893`) and per-restore `recoverInterruptedRestore(canonicalRoot)` (`486`). <br>**Beacon transfer protocol** `transfer/protocol.go:424-520` has true byte-offset resume for node-to-node migration: `AppendDestination(offset,total,checksum,body)` validates `offset==meta.Offset` (`435`), appends via `Seek(offset)` (`471-475`), tracks `meta.Offset += written` (`490`), `Head /api/v1/transfers/{id}/destination/archive` exposes current offset (`server.destinationTransferOffset`). Legacy `transfer/transfer.go:450-487` `streamToTarget` also supports `X-Transfer-Resume-Offset` header but legacy path rejects `resumeOffset !=0` (`119`). <br>**Forge API layer** restore has no offset resume; `restore.go:561-577` `Retry` resets `BytesProcessed=0, Progress=0` and re-executes whole restore with `maxRetries` loop (`460-481`). |

**Verdict:** Beacon's backup `.partial` is *not* resumable byte-offset; it's at-most-once with atomic rename. True resumable byte-stream exists only in `transfer/protocol.go` (migration), not backup/restore. Kopia/Restic rely on content deduplication for incremental efficiency, not offset resume.

---

### C-04 — Bandwidth / throttling controls

| Reference | Forge |
|-----------|-------|
| **Kopia** `repo/blob/throttling/` token-bucket throttler (`throttler.go:26-55`): per-operation buckets (`readOps`, `writeOps`, `listOps`, `upload`, `download`) + concurrent semaphores (`concurrentReads/Writes`), controllable via `PUT /api/v1/repo/throttle` and `PUT /api/v1/control/throttle` (`internal/server/server.go:151-152/189-190`) with `cli/throttle_set.go`. Also `internal/timetrack/throttle.go` throttles UI progress to 1/sec (`snapshotfs/snapshot_verifier.go:151`). <br>**Restic** backend-level throttling only via `backend/rclone` `wrappedConn` bandwidth limiting; no first-class Kopia-style throttle API. | **Beacon backup** `beacon/internal/backup/local.go:47/84-88` `writeLimit int64` + `SetWriteLimit(bytesPerSec)`; applied in `Create:292-295` via `rate.NewLimiter(rate.Limit(writeLimit), int(writeLimit))` wrapping writes to ZIP (`rateLimitedWriter:1015-1026` with `limiter.WaitN(ctx, len(p))`). Wired from `beacon/config/config.go:57/128` `backup.write_limit` (default `0=unlimited`) in `beacon/cmd/daemon/main.go:198-202` and on config reload `474-475`. S3 path delegates to same `local.SetWriteLimit` (`s3.go:63-65`). <br>**Beacon HTTP** tiered rate limiting via `beacon/internal/ratelimit/tiered.go` (`rateLimiter := ratelimit.NewTieredLimiter(rateTiers)` in `main.go:267`) — tiers `power=30, ws=60, files=120, default=240 req/min` (`main.go:269`), *not* bandwidth bytes/sec. Transfer protocol has no bandwidth limit. |

**Gap:** Forge throttles only `LocalBackup.Create` ZIP write bytes; S3 multipart upload (`s3.go:278-286`), transfer `AppendDestination` (`protocol.go:481`), and restore extraction are unthrottled. Kopia throttles at blob I/O layer per operation kind; Forge does one limiter total. No API to change `writeLimit` at runtime except SIGHUP reload.

---

### C-05 — Scheduling (cron / policy)

| Reference | Forge |
|-----------|-------|
| **Kopia** `snapshot/policy/scheduling_policy.go:64-72` `SchedulingPolicy{IntervalSeconds, TimesOfDay[], NoParentTimesOfDay, Manual, Cron[], RunMissed *OptionalBool}` with `NextSnapshotTime(previousSnapshotTime, now)` (`98-141`) computing earliest among interval (`previous+interval truncated`), next ToD (`getNextTimeOfDaySnapshot:144`), and `getNextCronSnapshot:169` using `hashicorp/cronexpr` (`Parse(stripCronComment(e))`). `checkMissedSnapshot:197` with 30-min grace and `RunMissed` flag; manual disables all. `ValidateSchedulingPolicy:276` rejects `Manual` combined with other fields. Policy merging via `Merge:227` with dedup (`SortAndDedupeTimesOfDay`). Scheduler drives via `internal/scheduler` ticker (`scheduler_test.go`). | **Beacon** `beacon/internal/backup/scheduler.go:15-98` `Scheduler` wraps `go-co-op/gocron` (`cron *gocron.Scheduler`), single map `jobs map[string]*gocron.Job` keyed by `serverID` (`42-73`); `Schedule` checks `validateServerRoot`, `adapters[name]` exists, rejects duplicate `serverID` (`55`), `RunBackup:100-130` writes `store.Create` with `BackupStatusCompleted`. No interval/TOD/manual — pure cron expr `gocron.Cron(cronExpr).Do`. <br>**Forge API — two schedulers:** (1) `forge/api/internal/services/backup/worker.go:15-199` `Worker` ticks every minute (`time.NewTicker(time.Minute):95`), loads `ListAllEnabledPolicies` (`119`), `policyDue(policy, now)` (`152-162`) returns `false` if `NextRunAt==nil` (first tick only initializes), else `!NextRunAt.After(now)`, then `executePolicy` (`201-220`) does server/volume/DB backup via `daemon.Client` and `persistNextRun:166-176` which parses via `NextCronRun` (`service.go:884-890` `cron.NewParser(Minute|Hour|Dom|Month|Dow)`) and stores `UpdateBackupPolicyNextRun`. Also `pickupBackupJobs:182-199` retries pending `ListRetryableBackupJobs(5)` with 20-min timeout. (2) `forge/api/internal/services/backup/config.go:351-360` `calculateNextCronRun` parses with `cron.ParseStandard`. QoL bug: `worker.go:161` comment explains `NextRunAt == nil` skips first tick to avoid immediate run. |

**Gap:** Forge API worker uses `robfig/cron` parser but only supports single `Interval`/`Cron` per policy vs Kopia's union of interval + multiple ToD + multiple cron entries + `RunMissed`. No equivalent of `NoParentTimesOfDay` inheritance or `Manual` mode. Beacon `scheduler.go` is standalone (not using `BackupPolicy` store) and is not integrated with worker — two disjoint schedulers exist (beacon-local `gocron` vs control-plane worker).

---

### C-06 — Hooks / actions (pre/post snapshot/restore)

| Reference | Forge |
|-----------|-------|
| **Kopia** `snapshot/policy/actions_policy.go:1-45` `ActionsPolicy{BeforeFolder, AfterFolder, BeforeSnapshotRoot, AfterSnapshotRoot ActionCommand}` with `ActionCommand{Command, Arguments, Script, TimeoutSeconds, Mode: essential|optional|async}`. Executed in `snapshot/upload/upload.go:603-608` via `executeBeforeFolderAction`/`executeAfterFolderAction` (`snapshot/upload/upload_actions.go:197-246`) inside temp WorkDir (`hc.WorkDir` via `MkdirTemp("kopia-action"):74`), env `KOPIA_ACTION/SNAPSHOT_ID/SOURCE_PATH/SNAPSHOT_PATH/VERSION` (`39-47`), captures `KOPIA_SNAPSHOT_PATH` stdout to redirect source (`captures map` `210-225`). | **Forge Beacon** no user-defined hooks. Only built-in lifecycle: `restoreBackup:1721-1722` hardcoded `pre-restore-*.zip` snapshot + `backupMu` serialize + `server.Create` failure path auto-rollback (`1732-1737`). `local.go:516-542` restore truncate path does staging swap, non-truncate overlays. <br>**Forge API** `restore.go:733-777` `createPreRestoreSnapshot` only if `RestoreOptions.CreateBackupBeforeRestore` is true (checked via unmarshal `736-740`), creates `pre-restore-<restoreName>-<ts>` artifact via `artifactService.Create` (`765`) and sets `CanRollback=true`/`RollbackArtifactID`. `RestoreOptions` itself supports `OverwriteExisting, StopAppBeforeRestore, StartAppAfterRestore, RestoreToOriginalLocation, CustomRestorePath, SkipVerification, CreateBackupBeforeRestore` (`70-81`) — but `daemonClient.RestoreBackup` fast path (`381-421`) ignores most options and always passes `false` for `truncate`. |

**Gap:** No Kopia-style arbitrary command/script hooks (essential/optional/async, timeout, stdout capture of snapshot path) pre/post backup or restore; Forge hardcodes only pre-restore snapshot. No folder-level `BeforeFolder/AfterFolder` granularity, no `AfterSnapshotRoot` for post-backup verification hooks.

---

### C-07 — Catalog / index / manifest

| Reference | Forge |
|-----------|-------|
| **Kopia** `repo/manifest` content-addressed manifests + `snapshot/manifest.go:13-240` typed `Manifest{ID manifest.ID, SourceInfo, StartTime, EndTime, RootEntry, Pins[], Policy}`; `snapshot/policy` inheritance tree; index via `repo/content` `index` (`content_manager` flush). Listing via `repo/manifest.Find` scoped by source. <br>**Restic** content-defined chunking → `internal/repository` pack files + `repo.LoadIndex` (`cmd_restore.go:160`) and `restic.Repository.LookupBlobSize` (`restorer.go:748`); index maps `BlobHandle{Type,ID}` → packs. Snapshot lookup via `SnapshotFilter.FindLatest(ctx, repo, repo, snapshotIDString)` (`cmd_restore.go:155`). | **Forge Beacon local** `beacon/internal/backup/local.go:26-32/413-443` stores `backup.zip` + `backup.zip.metadata.json` (`localMetadata{Checksum, Size, Created, IgnoredFiles}`) per namespace dir (`backupRoot/<namespace>/`). `List:415-440` does `os.ReadDir` + `readOrCreateMetadata` (backfills via `calculateChecksum` if missing:757-785), sorts by `Created`. No global index; S3 path lists via `ListObjectsV2Paginator` (`s3.go:170-206`) + `HeadObject` per key for checksum. <br>**Forge control-plane** `forge/api/internal/store/store_backups.go:10-401` Postgres `backups(uuid, server_id, name, checksum, size, status, is_locked, ... manifest, storage_receipt, checksum_verified, restore_count, last_restore_at, compressed, encrypted, nonce)` + `UpsertBackup` (`54-101`) with `ON CONFLICT (server_id, name)`. Policies in `backup_policies` table; `ListBackups/LookBackupPolicy` etc. No content-addressable manifest; `BackupManifest{Version,ChecksumAlgorithm,ChecksumValue,FileCount,TotalSizeBytes,SourceType,SourceID,Engine}` (`service.go:74-88`) is generated but not persisted as indexed manifest. |

**Gap:** Flat per-namespace directory listing vs content-addressable indexed catalog; no snapshot pin, no GC (`snapshotgc`, `snapshotmaintenance`), no global search across servers. Beacon's `List` does per-file `readOrCreateMetadata` + sort — O(N) `Stat` per backup; S3 list does N+1 HeadObject calls (one per object).

---

### C-08 — Resumable upload / download

| Reference | Forge |
|-----------|-------|
| **Kopia** upload resumability is via content-manager checkpoint (re-upload only missing contents); server interruptions resume via content index reconciliation. <br>**Restic** pack upload is not byte-resumable; repository lock serializes. | **Beacon local create** not byte-resumable (atomic rename — must restart). <br>**Beacon S3** `s3.go:126-161` `Create` first writes local staging (`local.Create`), then `uploadToS3` via `manager.NewUploader` (AWS SDK multipart) with 3-attempt exponential backoff (`retryBase time.Second`, `1<<(attempt-1)*delay`:139-158) but no byte-offset resume across daemon restarts (temp deleted via `defer local.Delete:135`). <br>**Beacon S3 download** `s3.go:288-375` `downloadToStaging` uses `CreateTemp(dir, ".s3-download-*.zip")` (`296`), streaming via `io.LimitReader(result.Body, maxS3DownloadBytes+1)` with `copyWithContext` (`338`), validates `expected` checksum from object metadata before download, disk-space check `ensureStagingDiskSpace` (`333`), post-download `calculateChecksum` compare (`361-368`), returns `stagedName + cleanup` closure — not resumable. <br>**Beacon transfer** `protocol.go:424-520` has true resumable `AppendDestination` (see C-03). Legacy `transfer.go:450-487` `streamToTarget` sets `X-Transfer-Resume-Offset` but legacy is `Deprecated` and rejects non-zero (`119-121`). |

**Gap:** No resumable S3 upload (multipart state not persisted) and no resumable backup `Create` — interrupted ZIP must fully restart. Only migration transfer has resume.

---

### C-09 — Progress reporting (poll vs WS vs callback)

| Reference | Forge |
|-----------|-------|
| **Kopia** `snapshot/restore/restore.go:95-110` `ProgressCallback func(ctx context.Context, s Stats)` invoked via `reportProgress` on `parallelwork.Queue.ProgressCallback` (`128-130`) and per-file `FileWriteProgress` (`235-238`) with atomic `statsInternal{RestoredTotalFileSize, EnqueuedTotalFileSize,...}:58-71`. `snapshot/snapshotfs/snapshot_verifier.go:151` throttles to 1/sec via `timetrack.Throttle`. <br>**Restic** `internal/restorer/progress_mock_test.go` / `internal/ui/restore/progress.go` terminal progress (`NewProgress` in `cmd_restore.go:170`), `Counter` for verify, `AddProgress(location, ActionXxx, size, total)` per node (`restorer.go:290,400,417,479`), `Finish()` at end (`cmd_restore.go:254`). | **Beacon** `beacon/internal/backup/backup.go:33-38` `BackupProgress{BytesProcessed, TotalBytes, Phase}` + `ProgressFunc`. `local.go:111-118` `reportProgress` publishes to `progress` callback; `Create` reports `creating backup→archiving files→completed` (`241→273→411`); `Restore` reports `restoring backup→completed` (`471-472` defer). <br>**Beacon WS** `beacon/internal/server/server.go:61/1542-1543` wires `backups.SetProgressCallback(func(p){eventBus.Publish(BackupProgressEvent+":"+serverID, p)})` and handler `backupProgressWS:2048-2101` authenticates (`ScopeWebsocket|ScopeBackupDownload`:2062), subscribes `eventBus.Subscribe(BackupProgressEvent+":"+serverID)` (`2085`), streams `writer.Write(msg)` until context cancel. No throttling; every `reportProgress` call publishes. <br>**Forge API** `job.go:281-322` `Update` and store `UpdateBackupJobProgress` track `currentPhase/progressPercentage/bytesProcessed`; `restore.go:836/875/etc` persists `CurrentPhase` via `persistRestore` (DB) on each phase change — polling only. <br>**Forge Web** `forge/web/components/server/backups-view.tsx:46` polls via `useQuery` with `refetchInterval: 3000` when any backup `status==="pending"||"running"`; no WS subscription; backup progress WS is not consumed by web. |

**Gap:** Web polls every 3s; beacon has WS `backupProgressWS` but frontend never connects to it (gap between beacon capability and web consumption). No throttling on beacon publish path → many rapid events for large restores. Kopia/Restic both throttle UI.

---

### C-10 — Restore journal / crash recovery

| Reference | Forge |
|-----------|-------|
| **Kopia** `repo/manifest` + content log provides crash consistency via content-manager `Flush` + manifest persistence; no staging journal for restore destination (restore writes directly, parallel). <br>**Restic** restore writes directly via `filerestorer` with temp files per file (`packer_manager.go:44` writes temp files, `restoreFiles` materializes), no swap journal; `DryRun` mode skips writes. | **Beacon local restore (truncate)** `local.go:544-624` 3-phase journal: `prepared` (staging written, `writeRestoreJournal:575`), `live-moved` (after `Rename(canonicalRoot, rollback):587-594`), `activated` (after `Rename(parent/stagingBase, canonicalRoot):596-609`), with `journalCommitted` boolean hiding journal until durable, `syncDirectory(parent)` after each rename, retains `rollback` dir until `RemoveAll + Remove(journalPath)` succeeds (`616-622`). <br>**Beacon recovery** `recoverInterruptedRestore:931-999` validates journal paths are within `parent` and prefixed `"."+base+".restore-"/".rollback-"` (`954-957`), handles `activated` (prefer live, delete rollback or promote rollback if live missing) vs `prepared/live-moved` (prefer rollback) (`960-990`), reaps staging + journal, `syncDirectory`. `RecoverRestoreJournals:867-893` scans `serverDataRoot` for `.*.restore-journal.json` at boot (comment says before server reconstruction). <br>**Beacon transfer** `protocol.go:635-660` `activateWithRollback` uses `.transfers/<migrationID>/previous` as rollback store; `Cancel:713-757` can roll back `activated` restores by swapping `previous` back over `canonical`. <br>**Forge API** `restore.go:733-777` `createPreRestoreSnapshot` creates artifact copy via `artifactService.Create` but does not write a file-level journal; failure to persist snapshot is logged warn and ignored (`767`). Control-plane `worker.go:401-448` retention enforcement runs per-policy but has no journal. |

**Strength:** Beacon's file-level journal is arguably stronger than Kopia/Restic for crash consistency of destructive restores. Weakness: S3 temp `.s3-download-*.zip` staging files have no journal/crash reaping; interrupted S3 `downloadToStaging` leaves temp until next restore call's `CreateTemp` collision or manual cleanup.

---

### C-11 — Checksum / verification (pre-restore)

| Reference | Forge |
|-----------|-------|
| **Kopia** per-content SHA256 via repository content addressing; verification via `snapshot/snapshotfs/snapshot_verifier.go` comparing content hashes. <br>**Restic** `restorer.go:714-775` `verifyFile` with `LookupBlobSize` + `restic.Hash(buf)` comparison per blob, `failFast` mode, `trustMtime` fast path (`738`), parallel `nVerifyWorkers=8` (`618`) in `VerifyFiles:624-680` using `errgroup`. Also `cmd_restore --verify` post-restore. | **Beacon local Create** computes `calculateChecksum` (SHA256 `io.Copy` via `sha256.New`): `backup.go:98-110` after `Rename`, stores in `localMetadata.Checksum` (`406-408`) via `writeMetadata:787-817` (atomic `CreateTemp + Rename` + `syncDirectory`). <br>**Beacon local Restore** verifies before any destructive action: `actualChecksum := calculateChecksum(backupPath)` vs `metadata.Checksum` (`508-514`), `ErrChecksumMismatch` if not `EqualFold` (`512`). S3 `downloadToStaging:318-368` validates S3 object `checksumMetadataKey=sha256` from `HeadObject` metadata, hex-decodes, checks length `sha256.Size*2`, verifies staged file via `calculateChecksum` vs `expected`, writes new `localMetadata`. <br>**Beacon local List/Get/Download** also verify: `Get` via `readOrCreateMetadata` (`442-452`), `Download:626-643` re-checks `calculateChecksum` vs metadata before `os.Open`. <br>**Forge API** `service.go:438-489` `VerifyChecksum(data, expected)` and `VerifyChecksumFromStore`/`VerifyStorageReceipt`/`GenerateManifest`; `job.go:608-612` verifies after `CreateFromBackupResult`. But `restore.go:733-1152` execute paths (including `daemonClient.RestoreBackup` shortcut `381-421`) call `verifyDatabaseRestore`/`verifyAppRestore` which are **no-ops** (`1154-1166` just log) — no real post-restore verification when going via `beaconClient` legacy path. Only S3 download path does checksum. |

**Gap:** Post-restore verification is stub in control-plane (`verifyDatabaseRestore:1154`/`verifyAppRestore:1163` return nil unconditionally) when `beaconClient` path is used; direct `daemonClient` path completes restore without any checksum or file-count validation.

---

### C-12 — Staging lifecycle / temp file handling

| Reference | Forge |
|-----------|-------|
| **Kopia** temp `hc.WorkDir` per action via `MkdirTemp("", "kopia-action"):74` cleaned in `cleanupActionContext:248`. <br>**Restic** `internal/fileio/file_unix.go:9` `TempFile` already deleted on Unix via `O_CLOEXEC` `/tmp` patterns; `repository/packer_manager.go` writes `*.tmp` then renames. | **Beacon Create** `local.go:275` `OpenFile(partial, O_CREATE|O_EXCL|O_WRONLY, 0600)` → `temp.Name()` tracked, `committed=false` defer deletes if not renames (`281-285`), final `Rename(partial→backupPath)` (`391-393`), `writeMetadata` atomic via `CreateTemp(.metadata-*)→Rename` + `syncDirectory` (`792-817`). <br>**Beacon Restore (truncate)** staging is `MkdirTemp(parent, "."+base+".restore-")` (`546`), tracked via `stagingBase`+`cleanupStaging` boolean defer (`551-555`), closed FS (`565`), `os.RemoveAll(staging)` on failure paths, cleared `cleanupStaging=false` only after `Rename(stagingBase→canonicalRoot)` succeeds (`606`). Rollback dir is `"."+base+".rollback-<nanos>"` (`572`). <br>**Beacon S3** `s3.go:296-306` `CreateTemp(dir, ".s3-download-*.zip")` with `cleanup` func deleting both file and `.metadata.json`; S3 `Create:135` defers `local.Delete` (deletes staged zip+metadata after upload or on failure). <br>**Forge API** `restore.go:809-813/924-928/etc` per-execute `tempDir := filepath.Join(os.TempDir(), "gamepanel-restore-"+uuid.NewString())` (`1169-1171` `createTempRestoreDir`) with `defer os.RemoveAll(tempDir)` and `backupFile := filepath.Join(tempDir, artifact.Name)` → `artifactService.DownloadToFile`. |

**Gap:** Beacon's `MkdirTemp` staging dir and rollback dir are created with `0o700`/`0o750` but live inside `parent` which is `filepath.Dir(canonicalRoot)` — if daemon runs as different UID than server owner, `Rename` semantics differ across filesystem boundaries (requires same FS). S3 `downloadToStaging` temp files are single-tenant per namespace dir but no global temp reaper for crash leaves (only restore journal is recovered).

---

### C-13 — Catalog concurrency / locking

| Reference | Forge |
|-----------|-------|
| **Kopia** repository lock (exclusive/shared via `repo/manifest` + `format/upgrade_lock.go:97`). <br>**Restic** `internal/repository` `LoadIndex` exclusive lock via `openWithReadLock` (`cmd_restore.go:149`). | **Beacon** `local.go:90-109` `namespaceOperation` per-namespace ref-counted `sync.Mutex` (`lockNamespace` increments `refs`, locks `operation.mu`, unlock decrements and deletes when 0). Used in `Create:239` and `Restore:469`. Plus `server.go:540-564` `backupMu sync.Mutex` global serializes `createBackup` and `restoreBackup` (`backupMu.Lock()` around pre-restore snapshot + restore). |
| **Forge API** `job.go:327-343` `ClaimBackupJobForExecution` atomic CAS (`UPDATE ... WHERE status IN ('pending','failed')`) prevents double execution by `worker.pickupBackupJobs` vs manual calls; restore has no equivalent CAS — `RestoreService.Execute:354-405` only checks `status != pending && status != failed` then sets `running`, so concurrent `Execute` calls race. |

**Gap:** Restore missing atomic claim; two concurrent `Execute("pending")` can both pass the check and interleave `persistRestore` writes (last-write-wins). Beacon's two-level locking (global `backupMu` + per-namespace) is sound but held for entire S3 download+extract, blocking concurrent backup creates for other servers.

---

### C-14 — Catalog retention / cleanup (scheduling tie-in)

| Reference | Forge |
|-----------|-------|
| **Kopia** `snapshotgc` + `snapshotmaintenance` with policy-driven retention (`snapshot/policy/retention_policy.go` — keep-last/N, hourly/daily/weekly/monthly/yearly, `snapshotmaintenance/snapshotmaintenance_test.go`). GC removes unreferenced contents after grace period. <br>**Restic** `cmd_prune` + `checker` for GC; retention via `forget` policies. | **Beacon** no retention; all backups remain until `DeleteBackup`. <br>**Forge API** retention at two layers: `worker.go:401-448` `enforceRetentionBeforeBackup` per-policy (count `MaxBackups` vs age `RetentionDays`, deletes oldest `sort.SliceStable` by `CreatedAt`:433) and `service.go:689-784` `CleanupExpiredBackups`/`EnforceRetentionPolicy` (double retention path). `store_backups.go:242-302` `CleanupOldBackups/CleanupOldBackupsForServer` respects `is_locked`. Policies store `RetentionDays`, `MaxBackups`, `Storage`, `Compress`. |

**Gap:** Beacon itself has no retention; retention is only control-plane and operates only on `backups` DB rows via `DeleteBackupFromStorage` → adapter `Delete`; if control-plane down, beacon accumulates untracked disk usage. No Kopia-style content GC — each backup is independent ZIP, no deduplication, so retention deletion saves exactly one ZIP, not just unreferenced blobs.

---

## 2. Logic Findings (4)

### LF-01 — Restore rollback supervision leak: pre-restore snapshot failure path skips rollback attempt coordination

**Severity:** P1 — data-loss risk on truncated restore  
**Location:** `beacon/internal/server/server.go:1720-1744` `restoreBackup`

```go
rollbackName := "pre-restore-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".zip"
s.backupMu.Lock()
_, snapshotErr := s.backups.Create(r.Context(), root, serverID, rollbackName, nil)
if snapshotErr == nil {
    // ... Restore ...
}
if err != nil {
    rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Minute)
    rollbackErr := s.backups.Restore(rollbackCtx, serverID, rollbackName, root, true, nil)
```

**Condition:** `snapshotErr != nil` falls through to `if err != nil` where `err` is still `nil` (restore never ran), so outer `if snapshotErr != nil` block logs and returns 500 without ever attempting to inspect whether a partial `Create` already committed the ZIP before failing on `writeMetadata`/`syncDirectory`. Stale `.partial` is cleaned but a half-committed `rollbackName` with bad metadata can remain; next restore with auto-generated `rollbackName` (timestamp) avoids collision, but `RollbackBackup` field returned on success (`1777`) is not exposed on failure path — caller cannot discover which rollback snapshot (if any) survived.

**Reference contrast:** Restic's two-pass restore never mutates source; Kopia's copier is non-destructive unless `--delete` — no pre-snapshot needed. Forge mandates pre-snapshot (correct) but error handling ignores its atomicity guarantees.

**Fix:** Check `os.Stat` / `Get` for `rollbackName` after `snapshotErr` and delete stale entry before returning; or create snapshot outside `backupMu` first, then acquire `backupMu` only for restore.

---

### LF-02 — S3 staged download checksum enforced, but local restore checksum source is untrusted `metadata.json`

**Severity:** P1 — integrity bypass  
**Location:** `beacon/internal/backup/local.go:757-785` `readOrCreateMetadata` + `508-514` restore verifier

```go
func readOrCreateMetadata(backupPath string) (localMetadata, error) {
    body, err := os.ReadFile(backupPath + metadataSuffix)
    if err == nil { // ... json.Unmarshal ... return metadata  ❌ no checksum recompute
    }
    if !os.IsNotExist(err) { return err }
    // backfill only when metadata missing
    checksum, _ := calculateChecksum(backupPath)
    metadata := localMetadata{Checksum: checksum, ...}
    writeMetadata(backupPath, metadata)
}
```

And in `Restore`:

```go
metadata, _ := readOrCreateMetadata(backupPath)
actualChecksum, _ := calculateChecksum(backupPath)
if !EqualFold(actualChecksum, metadata.Checksum) { return ErrChecksumMismatch }
```

**Issue:** If `metadata.json` already exists (normal case), `readOrCreateMetadata` trusts its `Checksum` verbatim without re-hashing — a tampered or stale sidecar (e.g., manual edit, bit-flip that preserves ZIP readability, or legacy migration `migrateLegacyBackups:212-218` that already calls `calculateChecksum` correctly but subsequent `writeMetadata` could be partial) will cause `Restore` to compare `actual` vs tampered `expected` and either false-pass (if attacker updated sidecar to match tampered ZIP) or false-fail (if sidecar is stale due to ZIP overwrite). For S3 downloads, `downloadToStaging:361-368` correctly validates `actual` vs `expected` from S3 object metadata *before* writing `localMetadata`; but local restore compares against a separate file that is not cryptographically bound to the ZIP (no HMAC).

**Reference contrast:** Kopia verifies by recomputing content hash over repository contents; Restic `verifyFile` recomputes `restic.Hash(buf)` per blob against `node.Content` (content-addressed), never relying on a sidecar. There is no trust-on-first-use sidecar.

**Fix:** On `Restore` and `Download`, ignore stored checksum and compute expected from ZIP itself vs a manifest embedded in storage receipt or use storage-provided ETag; or re-derive metadata on mismatch and fail closed with remediation log.

---

### LF-03 — Unthrottled `reportProgress` WS fan-out with no backpressure allows unbounded event queue growth

**Severity:** P2 — resource exhaustion / dropped updates  
**Location:** `beacon/internal/backup/local.go:111-118` `reportProgress` → `beacon/internal/server/server.go:1542-1543` `SetProgressCallback` → `events.Bus` (bounded channel) → `backupProgressWS:2085-2098`

```go
func (l *LocalBackup) reportProgress(..., phase string) {
    fn := l.progress
    if fn != nil { fn(BackupProgress{...}) }  // called per file in Create? No — only 3 phases
}
...
s.backups.SetProgressCallback(func(p backup.BackupProgress) {
    s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
})
...
ch := s.eventBus.Subscribe(...)
for { select { case msg, ok := <-ch: writer.Write(msg) } }
```

**Current behavior:** `Create` only calls `reportProgress` at 3 points (`creating backup`, `archiving files`, `completed`), so fan-out is bounded. But `Restore` for 10k-file ZIP calls `extractArchive:685-725` which does **not** call `reportProgress` per file — defer only sends `restoring backup` and `completed`. So today not a flood. However, `BackupManager` and `transfer.Engine` could call `reportProgress` more frequently if future callers update `BytesProcessed` per chunk; `events.Bus` is a bounded `chan []byte` (inspect `beacon/internal/events/bus.go`) with drop or block semantics — unbounded publishes with no throttle will either block the backup goroutine (deadlock under `backupMu`) or drop events for slow WS clients (no replay). Restic/Kopia throttle to 1/sec.

**Issue:** Adding per-file progress (natural improvement) will require throttling *before* `Publish`. Today silent success masks that architecture is not ready for granular progress.

**Fix:** Wrap callback with `timetrack.Throttle` (`ShouldOutput(time.Second)`) or token-bucket as Kopia does (`throttle.ShouldOutput(time.Second):snapshot_verifier.go:151`), or move `BackupProgress` to pollable store endpoint instead of WS flood.

---

### LF-04 — Control-plane restore `Retry` resets all state but does not delete the stale on-disk `DownloadToFile` temp dir if Execute is retried inline

**Severity:** P2 — disk leak / stale artifact reuse  
**Location:** `forge/api/internal/services/backup/restore.go:546-577` `Retry` → `Execute` loop + `809-813/924-928/1010-1014` `createTempRestoreDir`

```go
func (s *RestoreService) Retry(ctx context.Context, restoreID string, userID string) (*BackupRestore, error) {
    restore.Status = "pending"; restore.RetryCount = 0; ... BytesProcessed=0 ... BeaconTaskID=nil
    if err := s.persistRestore(ctx, restore); err != nil { ... }
    err = s.Execute(ctx, restoreID, userID)  // re-enters Execute synchronously
}
...
tempDir, _ := s.createTempRestoreDir() // MkdirAll(os.TempDir()/gamepanel-restore-<uuid>)
defer os.RemoveAll(tempDir)
backupFile := filepath.Join(tempDir, artifact.Name)
s.artifactService.DownloadToFile(ctx, artifact.ID, backupFile)
```

**Issue:** `Retry` and `Execute`'s own max-retries loop (`461-470` `restore.RetryCount++` then `Status=pending` with `_ = s.persistRestore`) both reuse the same `restoreID`. Each failed `executeAppRestore` call creates a new `tempDir` via `uuid.NewString()` and defers `RemoveAll`; successful defer runs on return. But if `Execute` fails and sets status back to `pending` without actually returning to caller (caller then retries via worker `pickupBackupJobs` or direct `Retry`), a second `Execute` invocation creates a new tempDir, but the prior `backupFile` streamed from S3/daemon may still be mid-download when context cancels — `DownloadToFile` has no resume, and orphaned `.tmp` inside tempDir is removed, but the DB `RestoreOptions` still point at no durable staging: S3 temp staging from `s3.go:296` is cleaned by deferred `cleanup()` *before* `local.Restore` returns, so there is no leak today. However, `createPreRestoreSnapshot:745-776` creates an artifact row (`rollbackArtifact`) on each retry attempt without cleaning prior rollback artifacts on failure — repeated retries accumulate `pre-restore-*` artifacts (`store.CreateRestoreJob` with new `rollbackName`).

**Reference contrast:** Kopia's content-manager checkpoint discards or reuses partially uploaded packs; not temp-dir per retry.

**Fix:** On retry, delete/limit prior rollback artifacts or add retention for failed restore attempts; document `MaxRetries` interacts with disk usage (each retry may allocate O(backup size) temp).

---

## 3. Additional Observations (non-blocking)

- **Beacon scheduler vs API worker overlap.** `beacon/internal/backup/scheduler.go` is never instantiated in `beacon/cmd/daemon/main.go` (only `backup.NewLocalBackup` is created). All scheduled backups go through API `worker.go:89-146` every minute. The beacon scheduler appears vestigial — remove or wire it to local policies for edge-offline operation. `reference/backup/kopia/internal/scheduler` drives per-source items with priority queue (`scheduler.Start(ctx, func→[]Item)`), not a single global ticker.
- **Blocked servers never restored.** `beacon/internal/backup/local.go:532-542` `truncate=false` overlays but leaves extra files; `truncate=true` does atomic swap and would discard extra files, equivalent to Restic `--delete`. However, `server.go:1729` `restoreBackup` calls `s.backups.Restore(..., body.Truncate, body.Paths)` where `Truncate` is client-controlled (`backups-view.tsx:65` hardcodes `false`), so full delete-extra restore is not exposed in UI — user cannot get Restic `--delete` semantics without API call. Kopia `DeleteExtra:104` and Restic `--delete` are prominently exposed.
- **Bandwidth limiter only on ZIP create.** `writeLimit` via `rate.Limiter` (`local.go:292-295` / `s3.go:63`) caps bytes/sec for `io.Copy` via `contextReader:828` but not for `extractArchive` writes during restore (which also do disk I/O). Transfer `AppendDestination:481` uses `io.Copy` with `LimitReader(remaining+1)` but no limiter. Kopia throttles at repo blob layer (both upload/download) symmetrically.
- **No progress in web.** Beacon has `GET /servers/{id}/ws/backup` (`server.go:351`) but `forge/web/components/server/backups-view.tsx:46` polls every 3s — no `useWebSocket`. Adding WS consumption would enable Kopia-style live progress vs poll delay.
- **Index warmed on every List.** `local.go:428-438` `List` calls `readOrCreateMetadata` for every `*.zip` (which may `calculateChecksum` if sidecar missing:776-782), so listing 100 backups after a storage crash computes 100 SHA256 sequentially. Kopia/Restic load a single index/manifest list. Consider lazy checksum or `sync.Pool` buffered hashing; S3 path already does `HeadObject` per item (N Head calls) — batch with `ListObjectsV2` size field instead.

---

## 4. Evidence Index (key files + lines)

| Claim | Evidence |
|-------|----------|
| Beacon full vs partial truncate logic | `beacon/internal/backup/local.go:468-542` |
| Path filtering + validation | `local.go:731-755`, `522-529` |
| Staging journal 3-phase | `local.go:544-624` + `899-928` + `931-999` |
| Crash recovery scan | `local.go:867-893` `RecoverRestoreJournals` |
| Checksum calc + sidecar | `beacon/internal/backup/backup.go:98-110`, `local.go:787-817`, `757-785`, `508-514`, `626-643` |
| Download checksum gate | `local.go:508-514` vs `s3.go:318-368` |
| S3 upload retry | `s3.go:139-160` exponential `1<<(attempt-1)` |
| S3 temp staging lifecycle | `s3.go:288-375` |
| Bandwidth writeLimit | `local.go:47/84-88/292-295/1015-1026`, `beacon/config/config.go:57/128`, `beacon/cmd/daemon/main.go:198-202/474-475` |
| Events WS | `beacon/internal/server/server.go:61`, `1542-1543`, `2048-2101` |
| Backup handler + pre-snapshot + rollback | `server.go:1691-1779` |
| Web polling vs WS gap | `forge/web/components/server/backups-view.tsx:43-48` |
| Control-plane restore Execute/Cancel/Retry/Rollback/Verify | `forge/api/internal/services/backup/restore.go:354-731`, `809-1152` |
| Job claim CAS vs restore no CAS | `job.go:337-343` `ClaimBackupJobForExecution` vs `restore.go:354-365` |
| Worker cron + policyDue | `worker.go:89-199`, `152-176` |
| Kopia scheduling policy union | `reference/backup/kopia/snapshot/policy/scheduling_policy.go:64-224` |
| Kopia actions hooks | `snapshot/policy/actions_policy.go:1-45`, `snapshot/upload/upload_actions.go:197-246` |
| Kopia throttle | `repo/blob/throttling/` (`throttler.go`), `internal/timetrack/throttle.go`, `internal/server/server.go:151-152` |
| Restic restorer SelectFilter + Overwrite + Delete | `reference/backup/restic/internal/restorer/restorer.go:31/49-59/164-273/345-552`, `cmd/restic/cmd_restore.go:195-238/92-96` |
| Restic verifyFile parallel | `restorer.go:618-680` `VerifyFiles` |
| Transfer offset resume | `transfer/protocol.go:424-520`, `transfer/transfer.go:450-487` |

---

## 5. Fix Priority

| Priority | Finding | Rationale |
|----------|---------|-----------|
| P1 | LF-01 (rollback supervision) | Data-loss on failed truncated restore if pre-snapshot not durable |
| P1 | LF-02 (checksum sidecar trust) | Integrity validation bypass if sidecar tampered |
| P2 | LF-03 (unthrottled WS progress) | Resource / backpressure; blocks future per-file progress |
| P2 | LF-04 (retry artifact accumulation) | Disk leak on repeated retry, unbounded `pre-restore-*` artifacts |

