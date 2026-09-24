# Subagent 09 — Backup/Storage: Crypto, Retention, Progress, StorageLocality — Implementation Plan

**Scope:** FINAL_PARITY §9 BK-01..BK-14 (re-verified still BROKEN). P0 BK-03 unauth sidecar transplant forgery, P0 BK-04 streaming OOM, P1 BK-06 retention AND-vs-OR inversion, P1 BK-07 prune without exclusive lock, P1 BK-08 orphan .partial sweep, P1 BK-09 in-process lock split-brain, P2 BK-13 dead progress WS, P2 StorageLocality vocab drift.

**Files cited:** `forge/api/internal/services/backup/encryption.go:92,150`, `beacon/internal/backup/local.go:757,275,90-109,415-440`, `beacon/internal/backup/retention.go:61-118`, `forge/api/internal/store/store_backups.go:240-302`, `beacon/internal/server/server.go:62,2155,1550`, `forge/api/internal/http/server.go:1995`, `forge/api/internal/http/realtime.go:124`, `forge/web/lib/api.ts:933`, `forge/api/internal/services/scheduler/service.go:212,317,703`, `forge/api/internal/placement/strategy.go:42`, `forge/api/internal/domain/domain.go:123`.

---

## 1. BK-03 — Unauthenticated Sidecar Transplant Forgery (P0) — Crypto Fix

### 1.1 Problem statement (re-verified)

- `forge/api/internal/services/backup/encryption.go:92` — `hkdf.Key(..., nil, purpose, ...)` uses `nil` salt. Every backup encrypted under the same master key derives the identical AES-256 key. Attacker with read access to any one backup ciphertext (e.g., leaked S3 object, old download) can transplant metadata/nonce? Actually current `Encrypt`/`Decrypt` derive deterministically; but the broader forgery vector cited is beacon-side: `beacon/internal/backup/local.go:757` `readOrCreateMetadata` recomputes `SHA-256(file)` as checksum via `calculateChecksum` (`beacon/internal/backup/backup.go:98`) and trusts it — unkeyed SHA, no AAD binding to `server_id:backup_name`. An unauthenticated sidecar process or a compromised adjacent namespace that can write a file into `backupRoot/<victim-namespace>/evil.zip` (path traversal guarded but symlink/TOCTOU aside) can then have its SHA accepted on restore (`local.go:512-513`). Forge-side similarly: `store_backups.go:65-95` stores `checksum` but never HMACs it against `FORGE_MASTER_KEY`.

- Second half: `beacon/internal/backup/local.go:757-782` `readOrCreateMetadata` is the sole checksum authority; `beacon/internal/backup/s3.go:318-328` validates S3 metadata `sha256` header but that header is attacker-controllable on upload if presupplied.

### 1.2 Desired end state

Streaming AEAD with **per-backup random 16-byte salt** + **AAD = `server_id:backup_name`** (Kopia pattern). Never reuse a key. Every encryption produces a distinct derived key via `HKDF-SHA256(salt, master, info)`. Decrypt fails if AAD mismatches — prevents transplant between servers / renames. Legacy backups remain readable via fallback header byte.

### 1.3 Migration SQL (additive, backward-compat)

```sql
-- 2026_08_30_backup_crypto_aad.sql — idempotent
ALTER TABLE backups
  ADD COLUMN IF NOT EXISTS enc_salt     BYTEA,          -- 16 bytes random per backup
  ADD COLUMN IF NOT EXISTS enc_aad     TEXT,           -- 'server_id:backup_name' at creation
  ADD COLUMN IF NOT EXISTS enc_version SMALLINT NOT NULL DEFAULT 1;
-- v1 = legacy HKDF(nil, purpose) + AES-GCM nonce-prefix, no AAD
-- v2 = HKDF(salt, master, purpose || \x00 || aad) + AES-GCM with AAD, streaming chunks
CREATE INDEX IF NOT EXISTS idx_backups_enc_version ON backups(enc_version) WHERE enc_version = 1;

-- Optional: track re-encryption progress (for async rewrap job, not blocking)
ALTER TABLE backups ADD COLUMN IF NOT EXISTS enc_rewrapped_at TIMESTAMPTZ;
```

No `NOT NULL` on `enc_salt` yet. v1 rows keep it NULL and `enc_version=1`.

### 1.4 Go — new crypto header & HKDF binding

**File:** `forge/api/internal/services/backup/encryption.go`

Current broken: `encryption.go:92` `hkdf.Key(sha256.New, masterKey, nil, purpose, ...)`

Replace with salt-aware derivation:

```go
const (
    encVersion1 byte = 0x01
    encVersion2 byte = 0x02
    saltSize    = 16
    // header: [version:1][salt:16][nonce:12] || ciphertext || tag (GCM handles tag inside Seal)
)

func deriveEncryptionKeyV2(masterKey, salt []byte, purpose, aad string) ([]byte, error) {
    if len(masterKey) == 0 { return nil, errors.New("master key empty") }
    if len(salt) != saltSize { return nil, errors.New("salt must be 16 bytes") }
    // info = purpose || 0x00 || aad  — mirrors Kopia's purpose separation, binds server_id:name
    info := purpose
    if aad != "" {
        info += "\x00" + aad
    }
    // HKDF-SHA256(salt=master_salt, IKM=masterKey, info)
    return hkdf.Key(sha256.New, masterKey, salt, info, aesKeySize)
}

func aadForBackup(serverID, backupName string) string {
    return serverID + ":" + backupName // stable, no normalization surprises; serverID is UUID, backupName is *.zip
}
```

**Streaming writer (Kopia-style chunked AEAD):**

Why not just `gcm.Seal(all)` — that is BK-04. New API:

```go
// forge/api/internal/services/backup/encryption.go

const chunkSize = 1 << 20 // 1 MiB plaintext per chunk — bounded memory

type StreamWriter struct {
    gcm    cipher.AEAD
    nonce  []byte // 12 bytes: 4 bytes fixed salt-derived prefix + 8 bytes counter (LE)
    ctr    uint64
    aad    []byte
    dst    io.Writer
    buf    []byte
}

func NewEncryptStreamWriter(dst io.Writer, masterKey []byte, serverID, backupName string) (*StreamWriter, func() (salt []byte, noncePrefix []byte, err error), error) {
    salt := make([]byte, saltSize)
    if _, err := rand.Read(salt); err != nil { return nil, nil, err }
    aad := aadForBackup(serverID, backupName)
    derived, err := deriveEncryptionKeyV2(masterKey, salt, purposeEncryptionKey, aad)
    if err != nil { return nil, nil, err }
    gcm, err := newGCM(derived)
    if err != nil { return nil, nil, err }
    noncePrefix := make([]byte, 4)
    if _, err := rand.Read(noncePrefix); err != nil { return nil, nil, err }
    nonce := make([]byte, nonceSize)
    copy(nonce[:4], noncePrefix)
    // header is written by caller: version + salt + noncePrefix
    // caller must write: []byte{encVersion2} || salt || noncePrefix  (21 bytes) before first Seal
    sw := &StreamWriter{gcm: gcm, nonce: nonce, aad: []byte(aad), dst: dst, buf: make([]byte, 0, chunkSize)}
    headerFn := func() ([]byte, []byte, error) { return salt, noncePrefix, nil }
    return sw, headerFn, nil
}

func (w *StreamWriter) Write(p []byte) (int, error) {
    // buffer and flush full chunks
    // ...
}
func (w *StreamWriter) flushChunk(final bool) error {
    // nonce = prefix(4) || ctr(8 LE)
    // binary.LittleEndian.PutUint64(w.nonce[4:], w.ctr)
    // sealed := w.gcm.Seal(nil, w.nonce, chunk, w.aad)
    // write length-prefixed: uint32 BE len(sealed) + sealed
}
func (w *StreamWriter) Close() error { return w.flushChunk(true) }

// Symmetric: StreamReader reads header, derives key, then loops length-prefix + Open
```

Keep legacy `Encrypt`/`Decrypt` for `enc_version=1` reads (backward compat). New code path writes `enc_version=2`.

**Beacon side** (`beacon/internal/backup/local.go:401-405`, `s3.go:272-286`) already writes/validates SHA; now also persist sidecar JSON with HMAC:

```go
// beacon/internal/backup/local.go — extend writeMetadata
type localMetadata struct {
    Checksum     string `json:"checksum"`
    HMAC         string `json:"hmac,omitempty"` // hex(HMAC-SHA256(master-derived, checksum || server_id || name))
    EncSalt      string `json:"enc_salt,omitempty"` // base64, v2 only
    EncVersion   int    `json:"enc_version"`
    Size         int64  `json:"size"`
    Created      time.Time `json:"created"`
    IgnoredFiles []string  `json:"ignored_files,omitempty"`
}
```

But beacon doesn't have `FORGE_MASTER_KEY`. So beacon HMAC must be **optional**; the authoritative AAD check happens forge-side on re-ingest (daemon streams ciphertext → forge decrypts with AAD check; if HMAC missing on local-only restores, warn but don't fail for v1). Document this split.

### 1.5 Backward compat & rollout (feature flag for per-blob HKDF)

- Flag: `BACKUP_ENCRYPTION_V2` (env, default `false` in first release, `true` next). When false, `EncryptReader` still uses old path (but instrumented to log that v2 would have been used). When true, new stream path is used.

- **Read path is forever dual:** `Decrypt` inspects first byte. If `0x02`, parse salt+noncePrefix+chunked framing and derive v2; if `0x01` or legacy magic (first byte of noncePrefix looks random, not 0x01/0x02), fall back to `deriveEncryptionKey(nil, purpose)` + single `gcm.Open(nil, nonce, ct, nil)`. No migration rewrites existing ciphertext; lazy rewrap on next backup/restore if desired (optional async job that re-encrypts v1→v2 when backup is next downloaded — not in critical path).

- **Tests:** `encryption_test.go` — vector for v2 round-trip, AAD mismatch must fail, salt uniqueness. Add `TestDecryptRejectsTransplantedAAD`.

---

## 2. BK-04 — Streaming OOM (`encryption.go:150`)

### 2.1 Re-verified

`EncryptReader` at `encryption.go:150,158` does `io.ReadAll(r)` then `encryptWithKey` then `io.Pipe` write of entire ciphertext. Same for `DecryptReader:186`. Any multi-GB backup (game world + mods) blows heap, OOM-kills beacon/API.

Simultaneous: `beacon/internal/backup/local.go:182-191` also does `io.Copy(temp, source)` without `io.CopyN` limit? Actually s3 staging `s3.go:338` does `io.LimitReader(..., maxS3DownloadBytes+1)` correctly, but encryption path lacks it.

### 2.2 Fix — wire `StreamWriter`/`StreamReader` above

```go
// forge/api/internal/services/backup/encryption.go

func EncryptReaderStreaming(r io.Reader, dst io.Writer, masterKey []byte, serverID, backupName string) (salt, noncePrefix []byte, err error) {
    sw, headerFn, err := NewEncryptStreamWriter(dst, masterKey, serverID, backupName)
    if err != nil { return nil, nil, err }
    salt, noncePrefix, _ = headerFn()
    // write header
    if _, err := dst.Write([]byte{encVersion2}); err != nil { return nil,nil,err }
    if _, err := dst.Write(salt); err != nil { return nil,nil,err }
    if _, err := dst.Write(noncePrefix); err != nil { return nil,nil,err }
    if _, err := io.Copy(sw, r); err != nil { return nil,nil,err }
    if err := sw.Close(); err != nil { return nil,nil,err }
    return salt, noncePrefix, nil
}

func DecryptReaderStreaming(r io.Reader, masterKey []byte) (io.Reader, string /*aadFromHeader or empty*/, error) {
    // Peek first byte to detect v1 vs v2
    // For v2: read salt(16)+prefix(4), derive key, return chunked reader that Opens each frame
    // For v1: legacy fallback — but still stream via limited buffering (do not ReadAll)
}

func EncryptReader(r io.Reader, key []byte) (io.Reader, error) {
    // DEPRECATED shim: if feature flag off, keep old behavior under size guard
    // else delegate to streaming with temp file? Prefer streaming API — callers must migrate
}
```

Call-site changes:

- `forge/api/internal/http/handlers_servers.go` backup download handler (if it uses `EncryptReader`) — switch to `io.Pipe` + `EncryptReaderStreaming` with serverID/name AAD.
- `beacon/internal/backup/s3.go:272 uploadToS3` — currently `os.Open(localPath)` then `manager.Upload` raw; with encryption, wrap file reader through `EncryptReaderStreaming` to S3 `Body`. Ensure `manager.Uploader` streams (it does multipart, but must not buffer whole file).
- `beacon/internal/server/server.go:1571 createBackup` — progress reporting must fire per-chunk, not just at start/end.

### 2.3 Memory bound & limits

- Chunk 1 MiB → peak ~1 MiB + GCM overhead (16 B) per goroutine. No `ReadAll`.
- Add global guard: refuse to `EncryptReaderStreaming` a source whose `Stat().Size()` pre-check > configured max (e.g., 100 GiB) unless streaming is confirmed; API returns `413`.
- Tests: `TestEncryptReaderStreaming_LargeDoesNotOOM` using `io.LimitReader(rand, 100<<20)` and `runtime.MemStats` delta < 10 MiB.

---

## 3. BK-06 — Retention AND vs OR Inversion (P1) — Data Loss

### 3.1 Re-verified

`beacon/internal/backup/retention.go:60-118` comment says `Intersection: a backup must satisfy ALL active rules to be kept` and initializes `keep` with `all`, then **intersects** with `MaxBackups` (`retention.go:105-118`). The SQL-side `store_backups.go:277-295` `CleanupOldBackupsForServer` similarly `AND ( created_at < now() - interval OR uuid IN (SELECT ... LIMIT ...) )` — the outer `AND` with lock/status correctly, but inner retention logic is `OR` (correct for deletion: delete if age-old OR over-limit). The bug is the **beacon retention.go inverting to AND for keeping**, which deletes more than intended.

Example: `MaxAge=30d, MaxBackups=10, KeepDaily=7`. A backup 31 days old but within newest 10 and within daily window should be **kept** under union-OR semantics (keep if ANY rule says keep). Under AND it is deleted — data loss.

`retention_test.go:15-66` expects 1 remaining with `MaxBackups=2, MaxAge=48h` — that test passes under AND but the expectation may itself encode the bug. Need to re-derive.

### 3.2 Desired semantics (converged OR union)

> **Keep iff (passes MaxAge) ∪ (falls in KeepDaily/Weekly/Monthly window) ∪ (in newest N by CompletedAt) ∪ (isLocked) ∪ (is most-recent safety rail).** Delete otherwise. If a rule is disabled (0), it contributes nothing to the union.

This is Kopia / Restic / Duplicati convention, least-surprising, least-destructive. Document explicitly in godoc and add `DecisionReason` for observability.

### 3.3 Go fix — `beacon/internal/backup/retention.go`

```go
// AFTER — union OR, converged across retention.go + store_backups + MainService

func (p RetentionPolicy) Apply(ctx context.Context, store Store, serverID string) error {
    // ... validation unchanged ...
    backups, err := store.List(ctx, serverID, 0)
    // ...
    sort.SliceStable(backups, func(i,j bool){return backups[i].CompletedAt.After(backups[j].CompletedAt)})
    now := time.Now()
    keep := make(map[string]bool, len(backups))

    // Track reason per backup for audit
    reasons := make(map[string][]string)

    // Rule 1: MaxAge — keep if NOT expired
    if p.MaxAge > 0 {
        for _, b := range backups {
            if now.Sub(b.CompletedAt) < p.MaxAge {
                keep[b.ID] = true
                reasons[b.ID] = append(reasons[b.ID], "max_age")
            }
        }
    }
    // Rule 2: KeepDaily/Weekly/Monthly — keep up to N per period
    // NOTE: previously this was buggy: it re-used keep map but counted incorrectly and skipped when max==0
    // Fix: keep the youngest N in each bucket, regardless of MaxAge result (union)
    for _, period := range []struct{ loHours, hiHours, max int; name string }{
        {0, 24, p.KeepDaily, "daily"},
        {24, 168, p.KeepWeekly, "weekly"},
        {168, 720, p.KeepMonthly, "monthly"},
    }{
        if period.max <= 0 { continue }
        count := 0
        for _, b := range backups {
            ageHours := int(now.Sub(b.CompletedAt).Hours())
            if ageHours >= period.loHours && ageHours < period.hiHours {
                if !keep[b.ID] { /* still keep below */ }
                // keep first N in this window (already sorted newest-first globally, but need per-window sort)
                // Better: collect window members, sort descending, keep top N
            }
        }
        // Correct per-window: collect then sort
    }
    // Simpler correct per-window: collect indices
    for _, per := range periods {
        var window []*Backup
        for i := range backups { if inWindow(backups[i], per) { window = append(window, &backups[i]) } }
        sort.Slice(window, func(a,b int){return window[a].CompletedAt.After(window[b].CompletedAt)})
        for i:=0; i < len(window) && i < per.max; i++ {
            keep[window[i].ID] = true
            reasons[window[i].ID] = append(reasons[window[i].ID], per.name)
        }
    }

    // Rule 3: MaxBackups — keep newest N
    if p.MaxBackups > 0 {
        for i, b := range backups {
            if i < p.MaxBackups {
                if !keep[b.ID] { keep[b.ID]=true }
                reasons[b.ID] = append(reasons[b.ID], "max_backups")
            }
        }
    }

    // If all rules disabled, keep everything (no deletion)
    if p.MaxAge==0 && p.MaxBackups==0 && p.KeepDaily==0 && p.KeepWeekly==0 && p.KeepMonthly==0 {
        for _, b := range backups { keep[b.ID]=true }
    }

    // Safety rail always
    keep[backups[0].ID]=true

    // Delete complement
    for _, b := range backups {
        if !keep[b.ID] {
            if err := store.Delete(ctx, b.ID); err != nil { return err }
        }
    }
    return nil
}
```

Full diff patch is ~40 lines; critical is removing the `maxKeep` intersection block (`retention.go:105-118`).

**Tests to add:**

- `TestRetention_UnionNotIntersection` — 4 backups: A 1h ago, B 2d ago, C 5d ago, D 40d ago. Policy `MaxAge=30d, MaxBackups=2`. Union keeps A,B,C? Actually MaxBackups keeps newest 2 (A,B), MaxAge keeps A,B,C — union keeps A,B,C (D deleted). Intersection would keep only A,B. Assert union.

- `TestRetention_KeepDailyUnionWithMaxAge` — backups spread daily over 10 days, `KeepDaily=2, MaxAge=3d`. Union should keep 2 newest daily + all within 3d (which overlaps but not same).

- Keep existing `TestRetentionPolicy` — update expectation to 2 remaining under union (A and B — B kept via MaxAge or MaxBackups), not 1. Flag test change in PR description to avoid reviewer confusion that the old expectation encoded the bug.

### 3.4 Forge-side convergence: `store_backups.go`

`CleanupOldBackups:242-259` and `CleanupOldBackupsForServer:263-302` must use **same union logic** as beacon, and must run inside advisory lock (see §4). Currently `CleanupOldBackupsForServer` does:

```sql
DELETE ... WHERE ... AND (created_at < now() - interval '1 day' * $2 OR uuid IN (...LIMIT ...))
```

That is **deletion predicate OR** — equivalent to union-keep's complement, so it's actually correct for deletion. But `count <= limit` short-circuit at `271` is wrong: it skips age-based deletion when under limit. Should always delete expired regardless of count. Fix:

```go
func (s *Store) CleanupOldBackupsForServer(ctx context.Context, serverID string, retentionDays int, backupLimit int) (int64, error) {
    // DO NOT early-return when count <= limit — still need to delete expired
    // Compute infinitely correctly via SQL below.
}
```

New SQL (union via OR, lock+status outer):

```sql
DELETE FROM backups
WHERE server_id = $1
  AND is_locked = FALSE
  AND status = 'completed'
  AND (
      ($2 > 0 AND created_at < now() - interval '1 day' * $2)  -- expired
      OR
      ($3 > 0 AND uuid IN (
          SELECT uuid FROM backups
          WHERE server_id = $1 AND is_locked=FALSE AND status='completed'
          ORDER BY created_at DESC
          OFFSET $3   -- keep newest $3, delete the rest (ordered oldest-last via OFFSET)
      ))
  )
-- For KeepDaily/Weekly/Monthly granularity, the per-period KeepX is not representable in pure SQL
-- without window functions. Decision: beacon handles granular policy; this fallback only handles
-- MaxAge + MaxBackups. Document that callers needing KeepDaily must use beacon RetentionPolicy.Apply.
```

Also need caller unification: `MainService` (wherever it calls `CleanupOldBackupsForServer`) and `store_backups.go` and `retention.go` must share a single `RetentionPolicy` struct via import or duplication with test that they are equal. Propose new shared type `internal/domain.RetentionPolicy` (or keep both but add compile-time assertion + property test that both produce same keep set for same input).

### 3.5 Convergence & config surface

- `forge/api/internal/services/catalog/retention.go` (found via glob) — likely duplicate retention for catalog? Unify or explicitly note it is out-of-scope and guards with same union test.
- Add `retention_invariants_test.go` that generates 100 random backup sets and random policies and asserts `beacon.Apply` keep-set == `forge.Cleanup` keep-set for MaxAge+MaxBackups subset.

---

## 4. BK-07 — Prune Without Exclusive Lock (SQL-only) + S3 Sweep

### 4.1 Re-verified

`store_backups.go:240-259` `CleanupOldBackups` and `263-302` `CleanupOldBackupsForServer` do plain `DELETE ... WHERE is_locked=FALSE AND ...` without `pg_advisory_lock`. Concurrent prune jobs (cron overlap, multi-replica API) can race with a `LockBackup`/`CreateBackup` and delete a just-locked row or violate backup-limit invariant. Also no S3 sweep.

### 4.2 Fix — advisory lock + S3 sweep marks

Pattern already in repo: `store.go:35 acquireMigrationLock`, `store_setup.go:107 pg_advisory_xact_lock`, `store_failover.go:136 pg_advisory_xact_lock(hashtextextended)`.

Add per-server backup prune lock:

```go
// forge/api/internal/store/store_backups.go

func backupPruneLockKey(serverID string) string {
    return "backup_prune:" + serverID // hashed via hashtextextended in SQL
}

func (s *Store) CleanupOldBackupsForServerLocked(ctx context.Context, serverID string, retentionDays int, backupLimit int) (int64, error) {
    tx, err := s.db.Begin(ctx)
    if err != nil { return 0, err }
    defer tx.Rollback(ctx)
    if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, backupPruneLockKey(serverID)); err != nil {
        return 0, fmt.Errorf("acquire backup prune lock: %w", err)
    }
    // 1) select doomed backups FOR UPDATE SKIP LOCKED (so we don't block live creates)
    rows, err := tx.Query(ctx, `
        SELECT uuid::text, name, storage_receipt
        FROM backups
        WHERE server_id=$1 AND is_locked=FALSE AND status='completed'
          AND (
            ($2>0 AND created_at < now() - interval '1 day' * $2)
            OR ($3>0 AND uuid IN (
                SELECT uuid FROM backups
                WHERE server_id=$1 AND is_locked=FALSE AND status='completed'
                ORDER BY created_at DESC OFFSET $3
            ))
          )
        FOR UPDATE SKIP LOCKED
    `, serverID, retentionDays, backupLimit)
    // collect, then attempt S3 sweep before DB delete
    // ...
    // 2) S3 sweep: for each doomed where storage_receipt indicates S3, call s3.DeleteObject with conditional
    //    On S3 failure, mark row with `status='prune_failed'` or set `storage_receipt->>'sweep_failed'=true` and SKIP that row's DELETE
    // 3) DELETE only the swept-ok rows
    // 4) commit
    return tag.RowsAffected(), tx.Commit(ctx)
}
```

- Global `CleanupOldBackups` (all servers) loops per-server and calls the locked variant (avoids one global lock which would serialize all servers). Add `SELECT DISTINCT server_id FROM backups WHERE is_locked=FALSE` then per-server lock.

- S3 sweep details (`beacon/internal/backup/s3.go:232 Delete`): needs to be idempotent; if object already gone, treat as success. Use `DeleteObject` with `IfMatch`? S3 conditional writes (new feature) not universally available — fallback to unconditional delete but verify after with `HeadObject` 404 means swept.

- Add metrics: `backup_prune_total`, `backup_s3_sweep_failed_total`.

### 4.3 Alternative for beacon local prune (if beacon also prunes local FS)

Beacon's `retention.go:35 Apply` deletes via `Store.Delete` which in daemon's SQLite path is local FS. That loop should also be under a file lock or SQLite `BEGIN EXCLUSIVE`. Simplest: wrap the whole `Apply` in `withPruneLock(serverID, fn)` that does `SELECT ... FOR UPDATE` equivalent on SQLite (`BEGIN IMMEDIATE`). Document.

---

## 5. BK-08 — GC Orphan `.partial` Never Indexed + Not Swept

### 5.1 Re-verified

`beacon/internal/backup/local.go:275` creates `backupPath+".partial"` staging file, and `List:415-440` filters `!validBackupName(entry.Name())` which rejects `.partial` (doesn't end in `.zip`). `List` also never returns `.partial` entries, so they are invisible to retention/prune and never GC'd on crash. `Create:279-286` defers `os.Remove(partial)` only if `!committed`, but if beacon crashes between `OpenFile .partial` and `Rename`, the file persists forever. `S3Backup.downloadToStaging:288-305` similarly creates `.s3-download-*.zip` with cleanup closure but no reaper for crash leftovers.

### 5.2 Fix — orphan `.partial` reaper

Two mechanisms:

**A) `List` already filters `.partial` — add explicit GC method, not indexing.**

```go
// beacon/internal/backup/local.go

const partialSuffix = ".partial"
const s3StagingPrefix = ".s3-download-"

// SweepPartialFiles removes *.partial and .s3-download-* older than maxAge.
// Called on startup and periodically (same schedule as retention).
func (l *LocalBackup) SweepPartialFiles(namespace string, maxAge time.Duration) (int, error) {
    dir, err := l.namespaceDir(namespace, false)
    if err != nil { return 0, err }
    entries, err := os.ReadDir(dir)
    if os.IsNotExist(err) { return 0, nil }
    if err != nil { return 0, err }
    now := time.Now()
    removed := 0
    for _, e := range entries {
        name := e.Name()
        isPartial := strings.HasSuffix(name, partialSuffix) || strings.HasPrefix(name, s3StagingPrefix)
        // also catch .metadata.json.tmp leftovers? pattern ".metadata-*"
        isMetaTmp := strings.HasPrefix(name, ".metadata-")
        if !isPartial && !isMetaTmp { continue }
        info, err := e.Info()
        if err != nil { continue }
        if now.Sub(info.ModTime()) < maxAge { continue }
        // Extra safety: check if file is open/locked by another Create (lsof is racy; instead use TryLock)
        // We do non-blocking flock via syscall.Flock with LOCK_EX|LOCK_NB, skip if locked
        if isFileLocked(filepath.Join(dir, name)) { continue }
        if err := os.Remove(filepath.Join(dir, name)); err == nil { removed++ }
    }
    return removed, nil
}

func isFileLocked(path string) bool {
    f, err := os.Open(path)
    if err != nil { return false }
    defer f.Close()
    // use syscall.Flock — best-effort, ignore on Windows
    return false // stub unless we add dependency
}
```

Simpler without flock: rely on age > 30 min (or 1h) — active writes are newer, so safe window. Default `maxAge=1h`.

**B) Global reaper on beacon start:**

```go
// beacon/internal/server/server.go — startup
if s.backups != nil {
    go func() {
        for _, ns := range listNamespacesOnDisk(backupRoot) { // enumerate subdirs
            if n, _ := s.backups.(*backup.LocalBackup).SweepPartialFiles(ns, time.Hour); n>0 {
                slog.Info("swept orphan partial files", "namespace", ns, "count", n)
            }
            if lb, ok := s.backups.(*backup.LocalBackup); ok {
                // also sweep S3 staging if S3 adapter wraps local:
            }
        }
    }()
}
```

**C) S3 staging similarly:** `s3.go:downloadToStaging` already has cleanup closure; add `S3Backup.SweepStaging(namespace, maxAge)` mirroring above for `.s3-download-*` leftovers (since S3Backup delegates `namespaceDir` to `local`).

**Tests:**

- `local_test.go` — add `TestSweepPartialFiles_RemovesOldPartialOnly` — create `.partial` with old mtime, fresh `.partial`, valid `.zip`, assert only old removed.

---

## 6. BK-09 — `lockNamespace` In-Process Only → Split-Brain

### 6.1 Re-verified

`beacon/internal/backup/local.go:48-55,90-109` `lockNamespace` is `sync.Mutex` per namespace in `namespaceOps` map. Multi-replica beacons (daemon scaled horizontally, or multiple beacons for different nodes sharing NFS `backupRoot`) have independent `namespaceOps` — no distributed exclusion. Two concurrent `Create` for same `namespace:name` can race: both `OpenFile .partial` with `O_EXCL` (good, second fails), but two different names can interleave `List`+`Delete` retention and cause lost-update / double-delete of S3 sweep (one deletes DB row while other is uploading).

### 6.2 Fix — distributed lock (DB row advisory_lock or S3 conditional)

Choose DB-backed as primary (beacon already has `Store` interface — can use SQLite `BEGIN EXCLUSIVE` for single-node SQLite, or pg advisory for shared postgres). For local FS only (no DB), fallback to `flock` on `backupRoot/<namespace>/.lock`.

**Option A: DB advisory (preferred when `Store` is postgres or shared):**

```go
// beacon/internal/backup/local.go — new interface

type DistributedLocker interface {
    TryLock(ctx context.Context, namespace string) (unlock func(), ok bool, err error)
}

// Postgres impl (beacon needs pgx pool — if unavailable, fallback to file lock)
type PGLocker struct{ db pgxPool }
func (p *PGLocker) TryLock(ctx context.Context, namespace string) (func(), bool, error) {
    // Use tx + SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))
    // Return unlock as rollback/commit handle
}
```

Simpler without new dependency: reuse existing `LocalBackup.lockNamespace` as fast-path, then layer file lock for cross-process:

```go
func (l *LocalBackup) lockNamespaceDistributed(namespace string) (func(), error) {
    // 1) in-process mutex (existing)
    unlockMem := l.lockNamespace(namespace)
    // 2) cross-process file lock
    dir, err := l.namespaceDir(namespace, true)
    if err != nil { unlockMem(); return nil, err }
    lockPath := filepath.Join(dir, ".backup.lock")
    fh, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
    if err != nil { unlockMem(); return nil, err }
    if err := syscall.Flock(int(fh.Fd()), syscall.LOCK_EX); err != nil {
        fh.Close(); unlockMem(); return nil, err
    }
    return func() {
        syscall.Flock(int(fh.Fd()), syscall.LOCK_UN)
        fh.Close()
        unlockMem()
    }, nil
}
```

- Replace calls in `Create:238` and `Restore:469` to use `lockNamespaceDistributed`. `List`/`Delete` are read-mostly; `Delete` already torques? Add lock to `Delete` to serialize with retention.

**Option B: S3 conditional (when S3 is authority):**

S3's `If-None-Match: "*"` on `PutObject` for lock key `locks/<namespace>.json` implements distributed lock without DB. Use AWS conditional writes (2024+). Probe availability via `HeadObject` fallback.

- Add feature flag `BACKUP_DISTRIBUTED_LOCK` (`flock` default, `pg` when beacon has postgres).

**Failure modes:**

- Stale file lock after crash: `Flock` is kernel-released on fd close / process death, so safe (unlike `.lock` file existence). No stale.
- NFS `flock` may not work; document that NFS backupRoot must be mounted with `lock` support or use DB lock; add startup warning if `flock` test fails.

---

## 7. BK-13 — Dead Progress WS (`beacon/server.go:2155` + `forge/api/server.go:1995`)

### 7.1 Re-verified

- Beacon publishes `BackupProgressEvent+":"+serverID` via `eventBus` (`server.go:1550-1551`, `local.go:111,241,273,411`). The WS handler `backupProgressWS:2155` subscribes and streams it.

- Forge proxies WS via `realtimeProxy` (`realtime.go:124`). `server.go:1995-2017` only mounts `stats|logs|console`:

```go
v1.Get("/servers/:id/ws/stats", ..., realtimeProxy(cfg, wsTickets, "stats"))
v1.Get("/servers/:id/ws/logs", ..., realtimeProxy(cfg, wsTickets, "logs"))
v1.Get("/servers/:id/ws/console", ..., realtimeProxy(cfg, wsTickets, "console"))
```

No `backup` route. Frontend `forge/web/lib/api.ts:933` `connectServerWebSocket` union is `"console" | "stats" | "logs"` — also excludes `backup`. So even if beacon WS exists, Forge never forwards it; ticket stream check `wsTicket.Stream != stream` (`realtime.go:167`) would reject `backup` ticket anyway because `IssueWSTicket` at `handlers_ws_ticket.go:165` defaults to `stream=c.Query("stream","console")` but validates `websocket.connect` — it would issue, but proxy has no route to consume.

Additionally early `TotalBytes` is unknown: `local.go:241 reportProgress(0,0,"creating backup")` sends 0/0 until completion. UI would show indeterminate spinner until done, not a real bar.

### 7.2 Fix — wire end-to-end (6 touchpoints)

**1) Forge proxy — add route:**

```go
// forge/api/internal/http/server.go — near line 1995
v1.Get("/servers/:id/ws/backup", requireRealtimeServices(cfg), wsOriginMiddleware(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "backup"), fiberws.Config{
    RecoverHandler: ...,
    Origins: wsUpgraderOrigins(getWebSocketAllowedOrigins(cfg)),
}))
```

**2) Ticket stream union — accept `backup`:**

No code change strictly required — `IssueWSTicket:165 stream := c.Query("stream","console")` already accepts any string and `realtimeProxy:167` checks `wsTicket.Stream != stream` equality, so `backup` ticket will match `realtimeProxy(..., "backup")`. But add explicit allowlist with `websocket.connect` scope:

```go
// handlers_ws_ticket.go — IssueWSTicket
allowedStreams := map[string]bool{"console":true,"stats":true,"logs":true,"backup":true}
if !allowedStreams[stream] {
    return fiber.NewError(fiber.StatusBadRequest, "unsupported stream: "+stream)
}
```

Also ensure `realtimeProxy` auto-mints beacon `wsToken` with correct stream — `daemon.MintWebsocketToken(target.NodeToken, serverID, userID)` currently doesn't encode stream; beacon's `authenticateWebSocket:2162-2173` validates token `ScopeWebsocket|ScopeBackupDownload` but not stream. Need stream in token claims? Beacon's `authenticateWebSocket` should check `claims.Stream == "backup"` or allow any if `ScopeBackupDownload`. Minimal fix: mint with same token, beacon accepts if `claims.Stream==""` or matches `r.PathValue("id")+"/*"`? Add stream to JWT claims for strictness.

**3) Beacon route — already exists at `server.go:352 GET /servers/{id}/ws/backup`** — confirm it is mounted when `s.backups != nil`. Verify `server.go:2155 backupProgressWS` uses `websocketUpgrader` with same `AllowedOrigins` as other WS handlers.

**4) Frontend union — `forge/web/lib/api.ts:933`:**

```ts
// BEFORE
export async function connectServerWebSocket(
  serverId: string,
  stream: "console" | "stats" | "logs",
): Promise<WebSocket> { ... }

// AFTER
export type ServerStream = "console" | "stats" | "logs" | "backup";
export async function connectServerWebSocket(
  serverId: string,
  stream: ServerStream,
): Promise<WebSocket> { ... }

// convenience
export function connectBackupProgressWS(serverId: string): Promise<WebSocket> {
  return connectServerWebSocket(serverId, "backup");
}
```

**5) Early TotalBytes via Stat:**

`local.go:273` currently `reportProgress(0,0,"archiving files")`. Fix:

```go
// local.go Create — before WalkDir, pre-scan to compute totalBytes (size sum via Stat)
var totalBytes int64
if err := filepath.WalkDir(canonicalRoot, func(path string, d fs.DirEntry, err error) error {
    if err != nil || d.IsDir() { return err }
    if d.IsDir() { return nil }
    // reuse denylist check to count only included files
    // ...
    if info, e := d.Info(); e==nil { totalBytes += info.Size() }
    return nil
}); err != nil { /* log but don't fail */ }

// Then during WalkDir, track bytesProcessed and report per-file:
var processed int64
// inside file copy loop, after _, err = copyWithContext(...)
processed += info.Size()
l.reportProgress(processed, totalBytes, "archiving")
```

For compressed archives, `totalBytes` is uncompressed source total; progress is still meaningful as bytes read. Alternative: report `bytesProcessed` only and UI shows `bytes / totalBytes` if known else indeterminate.

Also wire `BackupProgress` fields `BackupID`/`Phase` so UI can correlate when multiple backups run concurrently (namespace lock prevents concurrency per namespace, but global progress bus is shared).

**6) API job progress bridge (optional but closes end-to-end):**

`store_backup_jobs.go:262 UpdateBackupJobProgress` already persists `bytes_processed`. The beacon's `SetProgressCallback:1550` should also persist to DB via `panelClient`? Proposal: beacon pushes progress to forge via `panelClient.SendBackupProgress` (new method) on each `reportProgress` call; forge updates `backup_jobs` row and rebroadcasts via its own `eventBus` to WS subscribers. This avoids beacon WS only — forge WS becomes source of truth including DB durability.

If not adding DB bridge yet, at minimum document that forge `realtimeProxy` forwards beacon WS bytes 1:1 (no DB write), acceptable for MVP.

### 7.3 Frontend wiring details

- New hook `useBackupProgress(serverId)` in `forge/web/lib/hooks/useBackupProgress.ts` that calls `connectServerWebSocket(serverId, "backup")`, parses `BackupProgress` JSON, exposes `{bytesProcessed, totalBytes, phase, pct}`.
- Invalidation: `queryKeys.backups.byServer(serverId)` invalidated on `phase==="completed"` or WS close with `status==="completed"` (mirrors `queryKeys.backups` invalidation already in `web/lib/api/query-keys.ts:66`).

---

## 8. StorageLocality Dead Vocabulary Drift (`scheduler/service.go:212,317`)

### 8.1 Re-verified

- `scheduler/service.go:212` Filter checks `req.StorageLocality == "local_only"` — literal string.

- `scheduler/service.go:703-737 nodeToCandidate` derives `storageLocality := "local"` vs `"shared"` based on `RuntimeProvider`. No `"local_only"` ever produced. So `r.StorageLocality != req.StorageLocality` at `317` is always true when req is `"local_only"`, causing `ScoreNodes` to always apply `-1e10` penalty, even to local nodes. The branch `else if r.StorageLocality == req.StorageLocality` at `320` never fires for `local_only` requests.

- Source of truth drift: `forge/api/internal/services/evacuationplanner/service.go:28-33` defines `StorageLocalOnly="local_only"`, `StorageReplicated="replicated"`, `StorageShared="shared"`; `domain/domain.go:123` is untyped string; `placement/strategy.go:42-53` also string.

### 8.2 Fix — single vocabulary, wire at `handlers_servers.go`

**Canonical type:**

```go
// forge/api/internal/domain/domain.go or new forge/api/internal/storage/locality.go
type StorageLocality string
const (
    StorageLocalOnly   StorageLocality = "local_only"
    StorageLocalOnlyAlt StorageLocality = "local" // alias for backward compat
    StorageReplicated  StorageLocality = "replicated"
    StorageShared      StorageLocality = "shared"
)
func NormalizeStorageLocality(s string) StorageLocality { ... }
```

But minimal fix keeps strings; unify mapping in `nodeToCandidate`:

```go
func nodeToCandidate(snapshot store.NodeCapacitySnapshot, node store.Node) placement.Candidate {
    // ...
    // BEFORE: storageLocality="local" if runtime != nfs/shared else "shared"
    // AFTER: map local -> local_only (canonical) to satisfy Filter + Score equality
    storageLocality := string(domain.StorageLocalOnly) // "local_only"
    if node.RuntimeProvider == "nfs" || node.RuntimeProvider == "shared" {
        storageLocality = string(domain.StorageShared) // "shared"
    } else if node.DataLocality == "replicated" { // if such field exists
        storageLocality = string(domain.StorageReplicated)
    }
    // Node that is local still satisfies "local_only" request; shared node does not.
    // Preserve backward alias: if caller sent "local", normalize to "local_only" at entry.
    return placement.Candidate{ StorageLocality: storageLocality, ... }
}
```

**Normalization at entry:**

```go
// scheduler/service.go — normalizeRequest or FilterNodes entry
if req.StorageLocality == "local" {
    req.StorageLocality = "local_only"
}
if req.StorageLocality != "" {
    switch req.StorageLocality {
    case "local_only", "shared", "replicated":
        // ok
    default:
        return nil, fmt.Errorf("invalid storageLocality %q", req.StorageLocality)
    }
}
```

**Fix FilterNodes:212 to use normalized value:**

```go
// BEFORE: if req.StorageLocality == "local_only" && node.RuntimeProvider != "" && node.RuntimeProvider != "local" {
if req.StorageLocality == string(domain.StorageLocalOnly) {
    // local_only means require local storage; shared/replicated nodes are excluded
    isShared := node.RuntimeProvider == "nfs" || node.RuntimeProvider == "shared"
    if isShared {
        s.recordPlacementRejection()
        continue
    }
}
```

**Fix ScoreNodes:317-323 to use normalized comparison + alias tolerance:**

```go
normalizedReq := normalizeStorageLocality(req.StorageLocality)
if normalizedReq != "" && normalizeStorageLocality(r.StorageLocality) != normalizedReq {
    r.Score -= 1e10
    reason += "; storage locality mismatch penalty"
} else if normalizedReq != "" {
    r.Score += 1e8
    reason += "; storage locality match bonus"
}
```

**Wire at `handlers_servers.go` (FIX: currently missing StorageLocality in CreateServer request):**

`handlers_servers.go` `POST /servers` at `~890` builds `domain.PlacementRequest{ RegionID, ... Runtime }` but never sets `StorageLocality`. The `CreateServerRequest` likely carries a `storageLocality` field from body that is ignored.

Add to request parsing:

```go
type CreateServerRequest struct {
    // ...
    StorageLocality *string `json:"storageLocality" validate:"omitempty,oneof=local_only shared replicated local"`
}

normalizedLocality := ""
if req.StorageLocality != nil {
    v := strings.ToLower(strings.TrimSpace(*req.StorageLocality))
    if v == "local" { v = "local_only" }
    normalizedLocality = v
}

// in clusterManager.CreateServer call
PlacementRequest: domain.PlacementRequest{
    // ...
    StorageLocality: normalizedLocality,
    Runtime: req.Runtime,
}
```

Also persist choice: `store_servers.go` needs `storage_locality` column (additive migration, default `local_only` for existing). Document default.

**Tests:**

- `scheduler/service_test.go` — add `TestFilterNodes_StorageLocality_LocalOnlyExcludesShared`, `TestScoreNodes_StorageLocality_MatchBonus`.
- `handlers_servers_test.go` — POST with `storageLocality:"local_only"` selects local node, `shared` selects shared.

---

## 9. Frontend: Backups Tabs (Jobs/Policies/Providers/Verifications) + Progress Bars

### 9.1 Current frontend inventory

- `forge/web/components/server/server-nav.tsx:21`, `server-tabs.tsx:24` show `Backups` tab (permission `backup.read`).
- `forge/web/lib/api/query-keys.ts:29-36` defines `backups.admin.configs/jobs/artifacts/restores/status` keys and `invalidateQueries` helper.
- `forge/web/lib/api.ts` has no typed `BackupJob`/`BackupPolicy` fetchers yet (grep needed).
- Admin nav has `Admin > Backups` (`admin-registry.ts:81`).

Need to detail 4-tab implementation.

### 9.2 Design

**Tabs (within `/servers/:id/backups`):**

| Tab | Data source | Component | Polling / WS |
|-----|-------------|-----------|--------------|
| **Jobs** | `GET /api/v1/backups/jobs?server_id=:id` → `store_backup_jobs.go:ListBackupJobs` | `JobsTable` with `ProgressBar`, status badge, retry count, duration | WS `backup` for live `bytesProcessed/totalBytes` + 5s polling fallback |
| **Policies** | `GET /api/v1/backups/policies?server_id=:id` → `store_backup_policies.go:ListBackupPolicies` | `PoliciesTable` + Enable/Disable, Lock toggle, NextRun | no WS |
| **Providers** | `GET /api/v1/backups/configs` or admin providers (S3/local) | `ProvidersCards` (local, S3) with health | status poll |
| **Verifications** | `GET /api/v1/backups/verifications` or `store_backups.go:GetBackup` + checksumVerified | `VerificationsTable` — checksum, HMAC, restore_count | manual verify button |

**Progress bar component:**

```tsx
// forge/web/components/server/backups/BackupProgressBar.tsx
type Props = { serverId: string; job: BackupJob };
export function BackupProgressBar({ serverId, job }: Props) {
  const { progress } = useBackupProgress(serverId); // WS hook from §7.3
  const pct = progress?.totalBytes ? Math.round(progress.bytesProcessed / progress.totalBytes * 100)
            : job.progress_percentage ?? (job.bytesProcessed && job.totalBytes ? Math.round(Number(job.bytesProcessed)/Number(job.totalBytes)*100) : null);
  const phase = progress?.phase ?? job.currentPhase ?? job.status;
  // indeterminate if totalBytes == 0 → shimmer bar
}
```

**State management:**

- `web/lib/api/query-keys.ts` already has keys — add `byJob`, `byPolicy` helpers.
- Invalidation on WS `completed` → `invalidateQueries({queryKey: queryKeys.backups.byServer(serverId)})`.
- Optimistic update for `UpdateBackupJobProgress` (if forge emits via same WS, merge into query cache).

**API layer (`forge/web/lib/api.ts`):**

Add:

```ts
export type ApiBackupJob = { id:string; status:string; bytesProcessed:number; totalBytes: number|null; currentPhase:string; progressPercentage:number|null; ... };
export type ApiBackupPolicy = { id:string; enabled:boolean; interval:string; maxBackups:number; retentionDays:number; ... };
export async function fetchBackupJobs(serverId:string): Promise<ApiBackupJob[]> { return apiFetch(`/backups/jobs?server_id=${encodeURIComponent(serverId)}`) }
export async function fetchBackupPolicies(serverId:string): Promise<ApiBackupPolicy[]> { return apiFetch(`/backups/policies?server_id=${encodeURIComponent(serverId)}`) }
export async function fetchBackupVerifications(serverId:string): Promise<ApiBackupVerification[]> { ... }
```

**Wire in `server-nav.tsx` / `server-tabs.tsx`:** no change — keep `backup.read` gate. Tabs are internal to Backups page.

**Empty states & errors:** show `No backups yet` CTA to create, permission-denied tooltips.

---

## 10. Migrations — Additive, Feature Flag for Per-Blob HKDF Rollout, No Data Loss

### 10.1 Ordered migration set

| # | File | DDL | Gating |
|---|------|-----|--------|
| 1 | `migrations/044_backups_crypto_aad.sql` | `enc_salt, enc_aad, enc_version, enc_rewrapped_at` (above) | always additive, no NOT NULL |
| 2 | `migrations/045_backups_retention_union.sql` | `storage_locality TEXT DEFAULT 'local_only'` on `servers` + `nodes` if not exists; index | always |
| 3 | `migrations/046_backups_prune_locks.sql` | no DDL, just function `backup_prune_lock_key(text)` helper if needed | always |
| 4 | — | `store.go:35 acquireMigrationLock` already handles serial apply | — |

All `IF NOT EXISTS`, rerunnable.

### 10.2 Feature flag `BACKUP_ENCRYPTION_V2`

- Env: `BACKUP_ENCRYPTION_V2` (`bool`, default `false` release N, `true` release N+1).
- When false: new `StreamWriter` code is compiled but not used; `Encrypt` still calls `deriveEncryptionKey(nil, ...)`, header is old. Log at `Info` on first use: `backup encryption v2 disabled, using legacy`.
- When true: `EncryptReaderStreaming` is default; `Decrypt` auto-detects both.
- Flag also gates `nodeToCandidate` storageLocality normalization? That fix is safe to always enable — not flagged.

### 10.3 Backward compat guarantees

- **Decrypt forever supports v1** — no data loss for old archives.
- **Prune SQL** new `DELETE` predicate is strictly more conservative (keeps more due to OR → deleted set is subset? Actually union keeps more, deletes fewer — safe). If behavior change suspected, add `DRY_RUN` mode that logs would-delete vs actually deletes.
- **Retention.test** expectation change is flagged in changelog.
- **WS `backup` stream** is additive — old clients that don't know `backup` are unaffected.

### 10.4 No-data-loss checklist

- [ ] No `DROP`/`ALTER TYPE` that removes v1 support.
- [ ] No `DELETE` without `is_locked=FALSE` guard.
- [ ] Safety rail `keep[backups[0].ID]=true` preserved.
- [ ] `SweepPartialFiles` only removes aged files with `.partial` suffix, not valid `.zip`.
- [ ] File lock `Flock` is advisory + released on crash; retention still converges.
- [ ] `EncryptReaderStreaming` writes header before any ciphertext so crash leaves partial with valid header but still `.partial` suffix until Rename.

---

## 11. Execution Order (phases) & Ownership

**Phase 0 — Tests that encode bugs (land first, expect red):**
- `beacon/internal/backup/retention_test.go` union tests, `beacon/internal/backup/local_test.go` partial sweep, `forge/api/internal/services/scheduler/service_test.go` locality, `forge/api/internal/services/backup/encryption_test.go` AAD.

**Phase 1 — Crypto + streaming (P0):** `encryption.go` header + StreamWriter/Reader, beacon `local.go` HMAC extend, migration 044. Feature-flagged writer.

**Phase 2 — Retention union (P1):** `retention.go` fix + `store_backups.go` SQL fix inside `CleanupOldBackupsForServerLocked`, shared invariant tests.

**Phase 3 — Locks + GC (P1):** `store_backups.go` advisory-xact lock wrapper + S3 sweep marks, `local.go` `.partial` reaper + `lockNamespaceDistributed` (flock).

**Phase 4 — WS progress (P2):** `server.go:1995` route + `handlers_ws_ticket.go` allowlist + `lib/api.ts:933` union + hook + `local.go` early TotalBytes scan.

**Phase 5 — StorageLocality vocab (P2):** `scheduler/service.go:212,317,703` normalization, `domain/domain.go` typed const, `handlers_servers.go` wiring + column migration.

**Phase 6 — Frontend tabs (P2):** `Jobs/Policies/Providers/Verifications` + `BackupProgressBar`, query keys, invalidation on WS.

**Phase 7 — Rollout:** enable `BACKUP_ENCRYPTION_V2=true` in staging, verify matrix §12, cut release.

---

## 12. Verification Matrix

| Finding | Before | After | How to verify |
|---------|--------|-------|---------------|
| BK-03 | `hkdf ... nil` + unkeyed SHA | per-backup 16B salt + `AAD=server_id:backup_name` chunked GCM | `TestDecryptRejectsTransplantedAAD` + e2e: create backup srvA, copy .zip to srvB namespace, GET must 422 HMAC mismatch |
| BK-04 | `io.ReadAll` OOM | `StreamWriter` 1MiB chunks | `TestEncryptReaderStreaming_LargeDoesNotOOM` (100MiB, heap delta < 20MiB) + `go test -run TestEncrypt -memprofile` |
| BK-06 | AND intersection deletes valid | Union OR keeps if any rule | `TestRetention_UnionNotIntersection` + `retention_invariants_test` for 100 random cases |
| BK-07 | SQL-only prune no lock | `pg_advisory_xact_lock(hashtextextended("backup_prune:<id>"))` + `FOR UPDATE SKIP LOCKED` + S3 sweep | concurrent `go test -race` with two goroutines calling `CleanupOldBackupsForServerLocked` same id — exactly one deletes |
| BK-08 | `.partial` invisible | `SweepPartialFiles` + startup reaper | create aged `.partial`, run sweep, assert removed; fresh `.partial` retained |
| BK-09 | in-process `sync.Mutex` | `Flock` on `backupRoot/<ns>/.backup.lock` (+ pg advisory when available) | launch two beacons (separate processes) racing `Create` same ns different names — no interleaved delete |
| BK-13 | no `backup` WS route | `server.go:1995 backup` + `lib/api.ts:933 backup` + early TotalBytes | `curl -i ws://.../servers/:id/ws/backup?token=...` receives `{"bytesProcessed":..., "totalBytes":..., "phase":"archiving"}` before completion; UI bar moves |
| StorageLocality | `local` vs `local_only` always penalizes | normalized `local_only` canonical, alias `local` → `local_only` | `TestFilterNodes_StorageLocality_*` green, `ScoreNodes` bonus not penalty for local request |

Add `make test-backups` target running `go test ./beacon/internal/backup ./forge/api/internal/services/backup ./forge/api/internal/services/scheduler ./forge/api/internal/store -run Retention|Sweep|StorageLocality|Encrypt`.

---

## 13. Open Decisions (require owner sign-off)

1. **Master key location for beacon HMAC** — beacon today has no `FORGE_MASTER_KEY`. Options: (a) forge signs checksum and beacon just stores it (current proposal: forge-side AAD is authoritative), or (b) distribute read-only derived HMAC key to beacon via provisioned secret. Proposal keeps (a) to avoid secret sprawl.
2. **Retention KeepDaily granularity window 0-24/24-168/168-720 hardcoded** — confirm desired windows vs calendar-day/week/month (differences matter around month boundaries). Current hard hours kept to minimize churn; calendar-aware fix is follow-up.
3. **`flock` on NFS** — if fleet uses NFS for `backupRoot`, document requirement for `nolock` avoidance or mandate DB advisory lock mode.

---

## 14. File Checklist (touched files)

- `forge/api/internal/services/backup/encryption.go:92,103-176` — salt+AAD + StreamWriter/Reader
- `beacon/internal/backup/backup.go:98` — optional HMAC helper (or keep checksum only)
- `beacon/internal/backup/local.go:48,90-109,241,275,407,415,757,815` — metadata HMAC field, progress, .partial, flock
- `beacon/internal/backup/s3.go:272,286,338` — streaming encrypt + conditional sweep
- `beacon/internal/backup/retention.go:60-118` — union OR
- `beacon/internal/backup/store.go:8` — interface if HMAC needed
- `forge/api/internal/store/store_backups.go:240-302` — union SQL + advisory lock
- `forge/api/internal/store/migrations/044_backups_crypto_aad.sql` + `045_backups_retention_union.sql` — additive DDL
- `beacon/internal/server/server.go:62,352,1550,2155` — backupProgressWS route already, ensure progress publish + distributed lock use
- `forge/api/internal/http/server.go:1995` — add `backup` WS proxy route
- `forge/api/internal/http/handlers_ws_ticket.go:165` — add `backup` to allowlist
- `forge/api/internal/http/realtime.go:124,167` — stream param handling + optional stream-in-token
- `forge/web/lib/api.ts:933` — `ServerStream` union + helper
- `forge/api/internal/services/scheduler/service.go:212,317,703,751` — vocab normalization
- `forge/api/internal/domain/domain.go:123` — typed `StorageLocality` (optional)
- `forge/api/internal/placement/strategy.go:42,53,58` — candidate type rename note
- `forge/api/internal/http/handlers_servers.go:890` — wire StorageLocality into placement request + persist
- `forge/web/components/server/backups/*` — new tab components + `useBackupProgress` hook
- `forge/web/lib/api/query-keys.ts:29` — ensure keys still correct
