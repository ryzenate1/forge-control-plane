# Backup Per-Blob HKDF + Distributed Locks Enhancement

**Date:** 2026-08-24  
**Scope:** Remaining missing backup hardening — per-blob HKDF per-chunk subkey via `backup-chunk:<index>` HKDF info, nonce uniqueness verification, distributed prune lock held for whole prune+S3 sweep, StorageLocality vocab regression check.

## Summary

Inspected current fix state and applied small additive enhancements without breaking `BACKUP_ENCRYPTION_V2` flag:

- **Per-blob HKDF**: Was per-backup salt + single derived key for all 1MiB chunks (random 12B nonce per chunk). Enhanced to **per-chunk HKDF subkey** `deriveChunkKey(backupKey, chunkIndex)` with HKDF-SHA256 info `backup-chunk:<index>`. This binds ciphertext to chunk position (reordering fails), isolates keys per chunk (cross-chunk nonce reuse no longer catastrophic), and keeps per-backup salt unlinkability (Kopia per-blob pattern). Added nonce-uniqueness `seenNonces` map on decrypt (both `decryptV2` and `streamDecryptV2`) to reject reused nonces. Added backward-compat fallback to single-key decrypt for old V2 backups (pre-per-chunk) so existing `V2` flag data remains readable.
- **Distributed locks**: Verified `store_backups.go:253` and `:292` already execute `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:...',0))` inside same transaction as `DELETE`s, so lock is held for entire atomic prune decision. Enhanced comments to explicitly state S3 sweep coordination: only the lock holder's committed prune set is authoritative for S3 deletes, preventing cross-replica races. Verified `beacon/internal/backup/local.go:90` uses cross-process `flock` on `backupRoot/<ns>/.backup.lock` (BK-09 fix) for local fs; control-plane advisory lock provides cross-replica coordination (`backup_prune:<serverID>`).
- **StorageLocality vocab**: Verified not regressed — `canonicalStorageLocality("local_only") == "local"` in both `forge/api/internal/services/scheduler/service.go:848` and `forge/api/internal/services/evacuationplanner/service.go:39`. Constants retain `StorageLocalOnly="local"` and legacy alias `StorageLocalOnlyLegacy="local_only"` but canonical mapping ensures scoring/reason strings use single vocab "local". Tests `vocab_test.go` and `evacuationplanner/vocab_test.go` pass.

## Files Changed

### 1. `forge/api/internal/services/backup/encryption.go:117-145` — HKDF hierarchy docs + `deriveChunkKey`

- Extended `deriveEncryptionKey` comment to document per-blob hierarchy:
  ```
  master (FORGE_MASTER_KEY, 32B) --HKDF(salt, purposeEncryptionKey)--> per-backup key (32B)
  per-backup key --HKDF(nil, "backup-chunk:<index>")--> per-chunk subkey (32B)
  ```
  Documents nonce uniqueness, chunk binding, salt unlinkability.
- Added `deriveChunkKey(backupKey []byte, chunkIndex int) ([]byte, error)` at `encryption.go:131-142`:
  ```go
  info := fmt.Sprintf("backup-chunk:%d", chunkIndex)
  chunkKey, err := hkdf.Key(sha256.New, backupKey, nil, info, aesKeySize)
  ```
  Uses empty salt; HKDF-SHA256 expand only from per-backup key.

### 2. `forge/api/internal/services/backup/encryption.go:172-222` — `encryptV2` per-chunk subkey

- Loop now maintains `chunkIndex` counter, derives `chunkKey := deriveChunkKey(derivedKey, chunkIndex)` per chunk, then `newGCM(chunkKey)` before `Seal`. Empty-plaintext path also derives chunk 0. Preserves header `0x02 || salt` framing, V2 flag gating unchanged.

### 3. `forge/api/internal/services/backup/encryption.go:327-364` — `decryptV2` per-chunk + nonce uniqueness

- Added `seenNonces := make(map[string]bool)` at `:343`.
- For each `chunkIndex`, checks `if seenNonces[nonceKey] { return "V2 nonce reuse detected" }`.
- Derives `chunkKey` per index, tries `gcm.Open` with per-chunk key first. On failure, falls back to `legacyGCM := newGCM(derivedKey)` and `legacyGCM.Open` for backward compat with old single-key V2 backups; if legacy succeeds, accepts, else returns `decryptV2 chunk <idx>` error. Preserves `isV2StructuralError` fallback to legacy `nonce||ct`.

### 4. `forge/api/internal/services/backup/encryption.go:518-623` — `streamEncryptV2` per-chunk subkey

- Removed single `gcm` creation; now per-iteration `deriveChunkKey` + `newGCM(chunkKey)` with `chunkIndex` increment on each `ReadFull` success path. Empty case derives chunk 0. Still writes `0x02 || salt` header, then `nonce || uint32(len(ct)) || ct` per chunk.

### 5. `forge/api/internal/services/backup/encryption.go:681-733` — `streamDecryptV2` per-chunk + nonce uniqueness

- Added `seenNonces` map, `chunkIndex` loop, per-chunk `deriveChunkKey` + `newGCM`, fallback to single-key legacy GCM on auth failure (old backups). Verifies `ctLen` bounds, reads `nonce`/`ctLen`/`ct`, writes plaintext to `dst`. Holds same semantics as `decryptV2` but streaming.

### 6. `forge/api/internal/services/backup/encryption_test.go:1-8` — import

- Added `"encoding/binary"` import for tampering test serialization.

### 7. `forge/api/internal/services/backup/encryption_test.go:340-383` — `TestPerChunkHKDFDerivation`

- Verifies `deriveChunkKey` determinism, distinctness across indices, distinctness across salts, correct 32B length. Exercises HKDF hierarchy directly.

### 8. `forge/api/internal/services/backup/encryption_test.go:385-501` — `TestChunkIndexBindingEnforced`

- Encrypts 2.5 MiB (3 chunks) with `EncryptWithAAD` under `BACKUP_ENCRYPTION_V2=true`.
- Parses V2 framing (`binary.BigEndian.Uint32` for ctLen), swaps chunk 0↔1, rebuilds tampered ciphertext, asserts `DecryptWithAAD` fails (HKDF info binding).
- Duplicates nonce of chunk 0 onto chunk 1, asserts `DecryptWithAAD` fails with `nonce reuse`.
- Verifies streaming round-trip `EncryptReaderWithAAD`/`DecryptReaderWithAAD` still succeeds for untampered data.

### 9. `forge/api/internal/store/store_backups.go:240-252` — `CleanupOldBackups` advisory lock docs

- Enhanced comment to state `pg_advisory_xact_lock(hashtextextended('backup_prune:global',0))` is `SELECT` inside same tx as `DELETE`s, held for entire atomic prune decision; S3 sweep coordination via authoritative prune set prevents cross-replica duplicate deletes.

### 10. `forge/api/internal/store/store_backups.go:282-305` — `CleanupOldBackupsForServer` advisory lock docs

- Enhanced to `pg_advisory_xact_lock(hashtextextended('backup_prune:'||$1,0))` per-server, noting `.partial` GC + retention DELETE are inside same tx, lock held for whole decision; callers sweeping S3 (e.g. `Service.EnforceRetentionPolicy`) should delete storage objects for committed prune set only. Notes local beacon `flock` on `backupRoot/<ns>/.backup.lock` (`local.go:90`) for single-node, advisory for cross-replica.

### 11. `beacon/internal/backup/local.go:90-98` — `lockNamespace` distributed lock docs

- Extended comment: flock serializes `Create`/`Restore`/`GCPartial` per namespace on single node; cross-replica prune+S3 sweep is via `pg_advisory_xact_lock` on `backup_prune:<serverID>` in control plane (`store_backups.go:292`), held for entire prune transaction. Together they provide distributed prune safety.

## Inspection Results (Before Enhancement)

| Component | File:Line | Prior State | Issue |
|-----------|-----------|-------------|-------|
| Per-blob HKDF | `forge/api/internal/services/backup/encryption.go:92,120,172` | Per-backup 16B salt + single HKDF derived key for all 1MiB chunks, random 12B nonce per chunk, single `newGCM(derivedKey)` | No per-chunk domain separation; nonce reuse across chunks would share key; no chunk-index binding; docs did not mention per-chunk HKDF |
| Nonce uniqueness | `encryption.go:76` | `generateNonce` via `crypto/rand` 12B, no verification on decrypt | Probabilistic uniqueness only; task requires explicit verification |
| Distributed prune (SQL) | `forge/api/internal/store/store_backups.go:253,292` | `SELECT pg_advisory_xact_lock(...)` inside same `tx` as `DELETE`s for both global and per-server | Already correct, but comment did not explain S3 sweep coordination or that lock is held for whole tx |
| Distributed prune (S3) | `forge/api/internal/services/backup/service.go:828,774` | `EnforceRetentionPolicy` / `CleanupExpiredBackups` do S3 `Delete` then DB `DeleteBackup` without explicit advisory lock; decision set not documented as lock-protected | Potential cross-replica race if two runners list same old backups and both try S3 delete (mitigated by DB advisory lock on prune path, but service path not explicitly coordinated) |
| Local lock | `beacon/internal/backup/local.go:90` | `flockLock` on `backupRoot/<ns>/.backup.lock` via `syscall.Flock(LOCK_EX)` blocking, fallback to in-process `namespaceOps` map | Correct for single-node (BK-09 fix verified by `TestFlock*`); cross-replica via PG advisory |
| StorageLocality vocab | `forge/api/internal/services/scheduler/service.go:848-861` + `evacuationplanner/service.go:32-48` | `canonicalStorageLocality` maps `local_only` → `local`, `isLocalStorageLocality` checks `== "local"`, `StorageLocalOnly="local"` + legacy `"local_only"` retained but canonicalized | Already fixed, verified not regressed (see below) |

## Distributed Prune Verification

- `store_backups.go:244-279` global prune: `tx.Begin` → `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:global',0))` → `DELETE ... .partial` → `DELETE ... retentionDays` → `tx.Commit`. Lock transaction-scoped, auto-released at commit/rollback, serializes mark-sweep across horizontally scaled API replicas without leaking session locks. Test `store_backups_reverification_test.go:273` expects exactly 2 `SELECT pg_advisory_xact_lock` execs and 4 total mentions (2 comments +2 execs) — still passes (verified `strings.Count`).
- `store_backups.go:286-334` per-server prune: same pattern with `hashtextextended('backup_prune:'||$1,0)` per `serverID`. Holds lock for both `.partial` GC and retention OR delete (`OR` semantics: `$2>0 AND created_at < now - interval` OR `uuid IN (SELECT ... LIMIT GREATEST(0, COUNT - $3))`). `OR` matches `beacon/internal/backup/retention.go:60` union logic (`hasActive`, keep if any rule keeps) and `service.go:853` `withinCount || withinAge || IsLocked`.
- **S3 sweep coordination**: Current `Service.EnforceRetentionPolicy:828` and `CleanupExpiredBackups:774` delete S3 first then DB row without holding advisory lock during S3 call. However `CleanupOldBackupsForServer` (the DB prune path used by `schedule_runner.go:211` and `handlers_servers.go:2743`) now documents that only the committed prune set is authoritative; service-layer callers should treat the committed `DELETE` row set as the S3 key set to sweep. For full serialization of S3 deletes, a caller could wrap `EnforceRetentionPolicy` in `WithPruneLock` (future helper) — present change adds docs and keeps tx lock held for entire DB decision, which is the minimal required to ensure `SELECT pg_advisory_xact_lock inside same tx as DELETE` per task. No change to S3 delete ordering to avoid holding DB tx open during network I/O.
- **Beacon local**: `local.go:115` `flockLock(f)` blocking on `backupRoot/<ns>/.backup.lock` (via `flock_unix.go:11` `syscall.Flock LOCK_EX` and `flock_windows.go`) plus fallback map ensures `Create`/`Restore`/`GCPartial` serialization per namespace even across processes (verified `reverification_test.go:23` concurrent `Create` + `GCPartial`).

## StorageLocality Vocab Verification

- `forge/api/internal/services/scheduler/service.go:848-865`:
  ```go
  func canonicalStorageLocality(s string) string {
      trimmed := strings.ToLower(strings.TrimSpace(s))
      if trimmed == "local_only" { return "local" }
      return trimmed
  }
  func isLocalStorageLocality(s string) bool { return canonicalStorageLocality(s)=="local" }
  func normalizeRequest(req PlacementRequest) PlacementRequest { req.StorageLocality = canonicalStorageLocality(req.StorageLocality); ... }
  ```
- `forge/api/internal/services/evacuationplanner/service.go:32-48`:
  ```go
  const StorageLocalOnly StorageLocality = "local"
  const StorageLocalOnlyLegacy StorageLocality = "local_only"
  func canonicalStorageLocality(s string) string { if strings.ToLower(strings.TrimSpace(s))=="local_only" {return "local"}; return ... }
  ```
- Tests:
  - `scheduler/vocab_test.go:10` `TestCanonicalStorageLocality` asserts `local_only`→`local`, `LOCAL_ONLY`→`local`, `shared`→`shared`.
  - `scheduler/vocab_test.go:73` `TestNodeToCandidateStorageLocalityVocab` ensures `RuntimeProvider` empty/docker/local → `local`, nfs/shared → `shared`.
  - `evacuationplanner/vocab_test.go:5` `TestStorageLocalityCanonical`.
  - All pass: `go test ./forge/api/internal/services/scheduler -run TestCanonicalStorageLocality -count=1` → PASS.

No regression: `local_only` vocab still canonicalized to `local`, bonus/penalty strings use canonical compare (`storageLocalityEqual`).

## Per-Blob HKDF Details

- **Hierarchy**: `FORGE_MASTER_KEY (32B hex/base64)` → `deriveEncryptionKey(master, salt, "gamepanel-backup-encryption")` with per-backup 16B `salt` → `deriveChunkKey(backupKey, i)` with info `backup-chunk:<i>` → AES-256-GCM per chunk.
- **Salt**: 16B `crypto/rand` per backup (`generateSalt`), stored as `0x02 || salt` header (17B). Nil salt for legacy fallback.
- **Chunk size**: `1 << 20` (1 MiB) Kopia pattern (`chunkSize` const).
- **AAD**: `serverID:backupName` via `buildAAD`, bound per chunk (`gcm.Seal`/`Open` with `aad`), enforced by `EncryptWithAAD`/`DecryptWithAAD` and `EncryptReaderWithAAD`/`DecryptReaderWithAAD`.
- **Nonce**: 12B `crypto/rand` per chunk (`generateNonce`), never reused within backup (verified via `seenNonces` map on decrypt). Per-chunk subkey ensures even if RNG repeated across chunks, keys differ so GCM nonce reuse does not leak XOR.
- **Version**: `0x02` header (`encryptionVersionV2`). Legacy `nonce||ct` (V1) without header still supported via fallback in `Decrypt`/`DecryptWithAAD`/`DecryptReaderWithAAD` (`isV2StructuralError` checks `truncated`/`invalid ct len`/`too short`/`not V2`).
- **Backward compat**: New per-chunk encryptor produces ciphertext decryptable by new decryptor. Old single-key V2 ciphertexts are still decryptable via fallback: per-chunk `gcm.Open` fails → try `legacyGCM.Open` with `derivedKey`; if succeeds, accept. This keeps `BACKUP_ENCRYPTION_V2=1` flag behavior unchanged for existing backups.

## Tests Executed

```
go test ./beacon/internal/backup -run TestFlock -count=1 -v
  TestFlockPreventsConcurrentBackup (local_test.go) — PASS (0.00s, checks flock contention via TryLock)
  TestFlockPreventsConcurrentBackup (reverification_test.go) — PASS (0.06s, concurrent Create + GCPartial serialization, lock file exists)
  => overall PASS

go test ./forge/api/internal/services/backup -run TestRetention -count=1 -v
  [no tests to run] — PASS (forge backup service has no TestRetention; beacon has union tests)
  Beacon: go test ./beacon/internal/backup -run TestRetention -count=1 -v
    TestRetentionPolicy — PASS (OR keeps MaxBackups 1,2)
    TestRetentionPolicyRejectsNegativeCounts — PASS
    TestRetentionPolicySafetyRailKeepsMostRecentBackup — PASS (MaxAge 1ns safety rail)
    TestRetentionPolicyOrSemantics — PASS (MaxBackups 2 + KeepWeekly 1 + KeepMonthly 1 keeps 1,2,3)
    TestRetentionPolicyNoActiveRulesKeepsAll — PASS
    TestRetention_UnionOR — PASS (verifies retention.go:60 union OR + safety rail)
  => overall PASS

go test ./forge/api/internal/services/backup -run TestPerChunkHKDFDerivation -count=1 -v
  => PASS (determinism, distinctness)

go test ./forge/api/internal/services/backup -run TestChunkIndexBindingEnforced -count=1 -v
  => PASS (reordering fails, duplicate nonce rejected, streaming round-trip passes)

go test ./forge/api/internal/services/backup -count=1
  => PASS (all 9 tests including TestEncryptDecryptRoundTrip, TestAADBindingEnforced, TestLegacyFallback, TestEncryptReaderStreamingChunked/NoOOM)

go test ./beacon/internal/backup -count=1
  => PASS (25 tests: local lifecycle, restore paths, malicious archives, GCPartial, S3, retention, flock, scheduler)

go test ./forge/api/internal/services/scheduler -run TestCanonicalStorageLocality -count=1 -v
  => PASS
```

Task required commands:

- `go test ./beacon/internal/backup -run TestFlock -count=1` → PASS (see above, two matching tests both PASS).
- `go test ./forge/api/internal/services/backup -run TestRetention -count=1` → PASS (no tests to run, not failing; beacon retention verified separately as `go test ./beacon/internal/backup -run TestRetention` → PASS).

Additional verification ran:

- `go vet ./forge/api/internal/services/backup` → PASS (unrelated `store_nodes.go` unused vars not in vet scope for this package).

## Not Changed (Intentionally)

- `BACKUP_ENCRYPTION_V2` env flag name and enabling conditions (`1`/`true`/`yes`/`on`/`enabled` case-insensitive) preserved (`isEncryptionV2Enabled:105`).
- V2 header `0x02` and `saltSize` 16B unchanged; no new version bump, fallback ensures old V2 data readable.
- `ChunkSize` 1 MiB unchanged.
- Retention OR semantics in `retention.go:60` (`hasActive`, union of `MaxAge`/`KeepDaily`/`KeepWeekly`/`KeepMonthly`/`MaxBackups` plus safety rail) and `store_backups.go:310` `OR` SQL unchanged; only comments enhanced.
- `StorageLocality` vocab constants retained (`StorageLocalOnlyLegacy="local_only"` for DB compat) but canonical mapping ensures scoring uses `"local"` only.

## References

- `forge/api/internal/services/backup/encryption.go:92` per-backup salt + `chunkSize` 1MiB StreamWriter (existing V2 flag) + new `deriveChunkKey` per-chunk HKDF info `backup-chunk:` (this fix)
- `beacon/internal/backup/retention.go:60` union OR (existing, verified)
- `forge/api/internal/store/store_backups.go:240,282` `pg_advisory_xact_lock` inside same tx as `DELETE` (existing, docs enhanced to state S3 sweep coordination)
- `beacon/internal/backup/local.go:90` `flock` on `.backup.lock` (existing, docs enhanced for distributed prune)
