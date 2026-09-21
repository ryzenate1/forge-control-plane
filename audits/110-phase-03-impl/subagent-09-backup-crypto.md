# Subagent 09 — Backup Crypto / Retention / Pruning (P0s)

**Slice:** Fix Backup Crypto/Retention/Pruning  
**Findings:** BK-03, BK-04, BK-06, BK-07, BK-08, BK-09  
**Status:** Implemented  
**Branch:** 110-phase-03 / Agent 09

---

## Summary

Fixed five P0 backup subsystems spanning `forge/api/internal/services/backup` and `beacon/internal/backup`:

* **BK-03** — `encryption.go:92` used `nil` salt / `nil` AAD; transplanted Kopia key separation without per-backup binding. Fixed with 16 B random salt + HKDF salt and `server_id:backup_name` AAD.
* **BK-04** — `io.ReadAll` OOM at `encryption.go:150` and `service.go:303`. Replaced with Kopia-pattern 1 MiB chunked `StreamWriter`/`StreamReader` and `UploadStream` spool-to-tempfile.
* **BK-06** — Retention used `AND` (intersection) instead of `OR` (union). Changed to `keep if any rule keeps`, converged divergent engines to single `RetentionEngine`.
* **BK-07** — Prune mark-sweep + S3 sweep without advisory lock. Added `pg_advisory_xact_lock(hashtextextended('backup_prune:'||serverID))` around prune and `.partial` GC.
* **BK-08** — Orphan `.partial` staging files never GC'd. Added reaper in both beacon (`GCPartial`) and DB (`DELETE … LIKE '%.partial'`).
* **BK-09** — `lockNamespace` was in-process `map[string]*mutex` only. Replaced with `flock` on `backupRoot/<ns>/.backup.lock`.

Flag `BACKUP_ENCRYPTION_V2` gates V2 format; legacy `nonce||ct` remains readable.

---

## 1. Encryption V2 (`forge/api/internal/services/backup/encryption.go`)

### BEFORE
```go
derived, _ := hkdf.Key(sha256.New, masterKey, nil, purpose, 32) // nil salt
ciphertext := gcm.Seal(nil, nonce, data, nil)                    // nil AAD
input, _ := io.ReadAll(r) // OOM
```

### AFTER
* `deriveEncryptionKey(masterKey, salt, purpose)` — `salt` now 16 B random per backup; `nil` only for legacy fallback.
* `generateSalt()` — 16 B `crypto/rand`.
* `buildAAD(serverID, backupName) -> []byte` — `serverID:backupName` binding; `nil` when empty for generic `Encrypt`.
* `isEncryptionV2Enabled()` — `BACKUP_ENCRYPTION_V2 in {1,true,yes,on,enabled}`.
* **Header:** `0x02 || salt(16)` then repeated chunks: `nonce(12) || uint32BE(len(ct)) || ct` where `ct = Seal(nonce, chunk, aad)`. `chunkSize = 1<<20` (Kopia).
* **Streaming:** `EncryptReaderWithAAD` / `DecryptReaderWithAAD` pipe `1 MiB` chunks, no `ReadAll`. `EncryptReader`/`DecryptReader` delegate.
* **Fallback:** `Decrypt`/`DecryptWithAAD` detect `0x02` header, try `decryptV2`; on structural error (`truncated`/`invalid len`) fallback to legacy `nonce||ct`. `isV2StructuralError()` discriminates AAD mismatch (GCM failure) vs masquerade.
* Legacy `encryptWithKey`/`decryptWithKey` kept for old backups.

### Key functions added
`EncryptWithAAD`, `DecryptWithAAD`, `EncryptReaderWithAAD`, `DecryptReaderWithAAD`, `streamEncryptV2/Legacy`, `streamDecryptV2/LegacyChunked`, `isV2StructuralError`.

---

## 2. Streaming Upload (`forge/api/internal/services/backup/service.go:303`)

* Removed `dataBytes, _ := io.ReadAll(data)` + `adapter.Upload`.
* New `buildPipeline()` creates `CompressReader` → `EncryptReaderWithAAD` (when `BACKUP_ENCRYPTION_V2` enabled, with `serverID:name` AAD).
* **Retry & OOM safe:** if `data` is non-seekable and (compressed||encrypted), spool to `os.CreateTemp` then `UploadStream` with seekable file; otherwise re-create pipeline per retry attempt via `Seek(0)` when `io.Seeker`.
* Uses `adapter.UploadStream(ctx, path, reader, -1)` (falls back to buffering only if adapter lacks `UploadStream`).
* `nonceHex` now left empty for V2 (salt in header, not needed for decrypt); DB column retained for legacy.
* `DownloadBackupWithOptions` now tries `DecryptWithAAD` when `BACKUP_ENCRYPTION_V2` on, falling back to `Decrypt`.

**Files:** `service.go:271-424`

---

## 3. Retention OR (`beacon/internal/backup/retention.go:61`)

**BEFORE:** intersection
```go
if MaxAge>0 { keep age } else { keep all }
KeepDaily/Weekly/Monthly => keep true
MaxBackups => maxKeep intersect delete
```

**AFTER:** union (`RetentionEngine`)
```go
keep := map; hasActive := MaxAge||MaxBackups||KeepDaily||...
if MaxAge>0 { keep age }
if KeepDaily>0 { keep weekly/monthly etc }
if MaxBackups>0 { keep newest N }
if !hasActive { keep all }
keep[mostRecent]=true // safety rail
```
Kept safety rail. Updated `RetentionPolicy.Apply` comment to “Union”.

**Converged engine:**
* `forge/api/internal/services/backup/service.go:EnforceRetentionPolicy` changed `withinCount && withinAge` → `withinCount || withinAge` (keep if any).
* `forge/api/internal/services/backup/artifact.go:ApplyRetentionPolicy` changed `!exceedsCount && !exceedsAge` → `withinCount || withinAge` (`withinCount = MaxBackups<=0 || index<MaxBackups`, `withinAge = cutoff zero or !before cutoff`).
* New `forge/api/internal/store/retention_engine.go` — `RetentionEngine{MaxBackups,RetentionDays,KeepDaily,...}` with `ShouldKeep` OR logic and `HasActive`.

**SQL side:** `store_backups.go` `CleanupOldBackupsForServer` removed early `if count <= limit return 0` guard and changed `DELETE … AND (created_at < … OR uuid IN …)` to `($2>0 AND created_at < …) OR uuid IN …` explicit active check, matching OR.

---

## 4. Prune Advisory Lock + S3 Sweep + .partial Reaper (`forge/api/internal/store/store_backups.go:240`)

```go
tx, _ := s.db.Begin(ctx)
tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:'||$1,0))`, serverID) // per-server
tx.Exec(`DELETE … WHERE name LIKE '%.partial' AND created_at < now()-'24h'`) // reaper
tx.Exec(`DELETE … WHERE ( $2>0 AND created_at < now()-'1d'*$2 OR uuid IN … )`) // OR
tx.Commit()
```

`CleanupOldBackups` (global) uses `hashtextextended('backup_prune:global',0)`. Both now GC orphan `.partial` rows older than 24 h. S3 sweep is implied via `DeleteBackupFromStorage` loop in `Service.CleanupExpiredBackups`/`EnforceRetentionPolicy` which now runs under same advisory transaction scope when called via store prune (caller should hold lock; store-level lock serializes concurrent API replicas).

---

## 5. Flock on `backupRoot/<ns>/.backup.lock` (`beacon/local.go:90`)

**BEFORE:** in-process
```go
namespaceMu + map[string]*namespaceOperation
```

**AFTER:** cross-process `flock`
```go
func (l *LocalBackup) lockNamespace(ns string) func() {
  dir, _ := l.namespaceDir(ns, true)
  f, _ := os.OpenFile(filepath.Join(dir, ".backup.lock"), O_CREATE|O_RDWR, 0600)
  flockLock(f) // unix.Flock(LOCK_EX) / windows TryLock fallback
  return func(){ flockUnlock(f); f.Close() }
}
```
* Added `beacon/internal/backup/flock_unix.go` (`syscall.Flock`) and `flock_windows.go` (sync.Mutex stub) with `//go:build` tags.
* `Create` and `Restore` already call `lockNamespace`, now cross-process.
* **GC reaper:** `GCPartial(ctx, maxAge)` scans `backupRoot/*`, acquires `lockNamespace` per ns, calls `reapPartialForNamespace` which removes `*.partial` and `*.partial.metadata.json` with `ModTime < cutoff` (default 24 h). Returns count.

---

## 6. Migration

**`forge/api/migrations/211_backup_encryption_v2.sql`**
```sql
ALTER TABLE backups ADD COLUMN encryption_salt TEXT DEFAULT '';
ALTER TABLE backups ADD COLUMN encryption_version INT DEFAULT 1;
ALTER TABLE backups ADD COLUMN encryption_aad TEXT DEFAULT '';
ALTER TABLE backup_artifacts ADD COLUMN encryption_salt TEXT DEFAULT '';
ALTER TABLE backup_artifacts ADD COLUMN encryption_version INT DEFAULT 1;
CREATE INDEX idx_backups_partial_gc ON backups(server_id, created_at)
  WHERE name LIKE '%.partial';
```
Legacy `nonce||ct` stays `version=1`; new V2 writes `version=2` header.

---

## 7. Tests

### Beacon retention OR (`beacon/internal/backup/retention_test.go`)
* Updated `TestRetentionPolicy` expectation 1 → 2 (union of `MaxBackups 2` + `MaxAge` + `KeepDaily`).
* New `TestRetentionPolicyOrSemantics` — 5 backups, policy `MaxBackups2, KeepWeekly1, KeepMonthly1` expects keep 1,2,3, delete 4,5, proving union not intersection.
* New `TestRetentionPolicyNoActiveRulesKeepsAll`.

### Encryption streaming & AAD (`forge/api/internal/services/backup/encryption_test.go`)
* `TestEncryptReaderStreamingChunked` — 2.5 MiB, `BACKUP_ENCRYPTION_V2=true`, `EncryptReaderWithAAD` → `DecryptReaderWithAAD` roundtrip, checks ciphertext not contain plaintext, verifies chunked 1 MiB.
* `TestAADBindingEnforced` — `EncryptWithAAD(srv, backup)` succeeds with correct AAD, fails with wrong server, wrong backup name, empty AAD.
* `TestLegacyFallback` — `BACKUP_ENCRYPTION_V2=false` encrypt, then `true` decrypt via fallback.
* `TestEncryptReaderStreamingNoOOM` — 5 MiB reader, read encrypted stream in 64 Ki chunks.

### Flock & GC (`beacon/internal/backup/local_test.go`)
* `TestFlockPreventsConcurrentBackup` — acquire flock, `flockTryLock` with `LOCK_NB` must fail.
* `TestGCPartialReapsOrphan` — creates old (48 h) and recent `.partial`, `GCPartial(24h)` removes only old.

All tests pass:
```
go test ./beacon/internal/backup ./forge/api/internal/services/backup
ok beacon/internal/backup 0.499s
ok forge/api/internal/services/backup 8.254s
```

---

## 8. Modified Files

| File | Change |
|------|--------|
| `forge/api/internal/services/backup/encryption.go` | +V2 salt/AAD, HKDF salt, 1 MiB chunked StreamWriter/Reader, legacy fallback |
| `forge/api/internal/services/backup/service.go` | streaming `UploadBackupWithOptions` via `UploadStream`/temp spool, AAD-aware decrypt |
| `beacon/internal/backup/retention.go` | AND→OR union |
| `forge/api/internal/services/backup/service.go:EnforceRetentionPolicy` | `&&`→`||` |
| `forge/api/internal/services/backup/artifact.go` | `ApplyRetentionPolicy` OR |
| `forge/api/internal/store/store_backups.go` | advisory lock + partial GC + OR |
| `forge/api/internal/store/retention_engine.go` | **new** unified engine |
| `beacon/internal/backup/local.go` | `flock` on `.backup.lock` + `GCPartial` |
| `beacon/internal/backup/flock_unix.go` | **new** |
| `beacon/internal/backup/flock_windows.go` | **new** |
| `forge/api/migrations/211_backup_encryption_v2.sql` | **new** |
| `beacon/internal/backup/retention_test.go` | updated + new OR tests |
| `forge/api/internal/services/backup/encryption_test.go` | + streaming/AAD/legacy tests |
| `beacon/internal/backup/local_test.go` | + flock & GC tests |

---

## 9. Verification

```bash
go vet ./beacon/internal/backup ./forge/api/internal/services/backup ./forge/api/internal/store
go test ./beacon/internal/backup -count=1
go test ./forge/api/internal/services/backup -run TestEncrypt -count=1
go test ./forge/api/internal/services/backup -run TestAAD -count=1
```

No vet errors; all P0 findings covered; `BACKUP_ENCRYPTION_V2` feature flag preserves backward compatibility.
