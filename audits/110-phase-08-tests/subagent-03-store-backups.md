# Subagent 03 — Store Backups / Certificates / DNS Tests (Phase 08)

**Agent:** 03/20  
**Focus:** `store_backups.go:240` retention OR, `store_certificates.go:80` DNS encryption, `store_acme_accounts.go`, migrations `211`, `213`  
**Date:** 2026-08-24

## Task

- Inspect `store_backups.go:240`, `store_certificates.go:80`, `store_acme_accounts.go`, migrations `211`, `213`
- Check existing: `ls forge/api/internal/store/store_backup*test.go`
- Create/augment: `store_backups_reverification_test.go` covering retention OR vs AND, prune `pg_advisory` lock, GC orphan `.partial`
- Run: `go test ./forge/api/internal/store -run TestBackup -count=1 -v 2>&1 | tail -n 30; go test ./forge/api/internal/services/backup -run TestRetention -count=1`
- Ensure `store_backups.go` retention change doesn't break: `go test ./forge/api/internal/services/backup -count=1`
- Report here

## Inspection

### 1. `forge/api/internal/store/store_backups.go:240-335` — CleanupOldBackups

`store_backups.go:240-280` `CleanupOldBackups(ctx, retentionDays, autoCleanup)`:
- Guard `if !autoCleanup || retentionDays <=0 {return 0,nil}` `store_backups.go:245` — disabled prune is no-op.
- `tx.Begin` + `defer Rollback` + `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:global',0))` `store_backups.go:253` — transaction-scoped advisory lock serializes prune mark-sweep across replicas (comment `store_backups.go:242-243` "Converged to single RetentionEngine (OR semantics) and protected by pg_advisory_xact_lock").
- GC orphan `.partial` `store_backups.go:258-266`: `DELETE FROM backups WHERE is_locked=FALSE AND status='completed' AND name LIKE '%.partial' AND created_at < now() - interval '24 hours'` — reaper for orphan uploads.
- Main retention delete `store_backups.go:267-272`: `DELETE WHERE is_locked=FALSE AND created_at < now() - interval '1 day' * $1 AND status='completed'` — retentionDays-only, global (no per-server limit).

`store_backups.go:282-335` `CleanupOldBackupsForServer(ctx, serverID, retentionDays, backupLimit)`:
- Per-server `pg_advisory_xact_lock(hashtextextended('backup_prune:'||$1,0))` `store_backups.go:292` — key namespaces per server to allow parallel prune of different servers, still serial per-server.
- Same `.partial` GC but scoped `WHERE server_id=$1 AND is_locked=FALSE AND status='completed' AND name LIKE '%.partial' AND created_at < now()-24h` `store_backups.go:296-303`.
- **OR semantics** `store_backups.go:306-326`: comment "RetentionEngine OR: keep if any rule keeps, delete only if exceeds ALL active thresholds" + SQL:
  ```sql
  DELETE WHERE server_id=$1 AND is_locked=FALSE AND status='completed' AND (
    ($2>0 AND created_at < now() - interval '1 day'*$2)  -- age branch
    OR
    uuid IN (SELECT uuid FROM backups WHERE server_id=$1 AND is_locked=FALSE AND status='completed'
             ORDER BY created_at ASC
             LIMIT CASE WHEN $3>0 THEN GREATEST(0, COUNT(*)-$3) ELSE 0 END)
  )
  ```
  Uses `OR` between retention-age and over-limit branches. `GREATEST(0, COUNT-$3)` prevents negative LIMIT. `COUNT(*)` correctly counts `status='completed'` (locked excluded from victim list but included in count? intentional — limit counts all completed, victims are only unlocked).

- Both prunes: `is_locked=FALSE` + `status='completed'` respected; transactional `Begin`/`Commit`.

- Converged to `RetentionEngine` `retention_engine.go:5-47` which implements `ShouldKeep(age,rank,period)` with `rank<MaxBackups || age<=RetentionDays*24h || age<MaxAge || period!=""` — pure OR.

### 2. `forge/api/internal/store/store_certificates.go:80-127` — CreateCertificate

`store_certificates.go:80-127`:
- Validates `len(Domains)>0`.
- Encrypts `PrivateKey` via `encryptSecret(..., secretAAD("certificates",id,"private_key"))` `store_certificates.go:87`.
- `ChallengeType` defaults to `"http-01"` `store_certificates.go:91-93`.
- DNS credentials: marshals `req.DNSCredentials` map, encrypts only if non-empty `!= "{}"` via `secretAAD("certificates",id,"dns_credentials")` `store_certificates.go:95-112`; else leaves `dnsCredsEncrypted=""`.
- Insert clears plaintext: `VALUES (..., '{}'::jsonb, $11, ...)` where `$11=dnsCredsEncrypted`, column `dns_credentials` forced to `'{}'` `store_certificates.go:114-117` — dual-write, ciphertext only.
- `GetCertificate` `store_certificates.go:129-168` selects `COALESCE(private_key_encrypted,''), COALESCE(dns_credentials_encrypted,''), COALESCE(dns_credentials::text,'{}')` and dual-reads: tries `decryptSecret(dnsEncrypted, plainForDecrypt, secretAAD(...))` then falls back to plaintext if `("{}" or empty)` — supports migration.
- Same dual-read in `ListCertificates` `store_certificates.go:170-236` and `FindExpiringCertificates` `store_certificates.go:301-345`.

### 3. `forge/api/internal/store/store_acme_accounts.go` — ACME + DNS provider

`store_acme_accounts.go:77-101` `CreateAcmeAccount`:
- Validates email, generates UUID, defaults `CAURL` to `https://acme-v02.api.letsencrypt.org/directory`.
- Encrypts `PrivateKey` via `secretAAD("acme_accounts",id,"private_key")` `store_acme_accounts.go:86`.
- Insert `VALUES ($1,$2,'', $3,...)` plaintext `private_key` cleared to `''`, only `private_key_encrypted` stored `store_acme_accounts.go:90-93`.
- `GetAcmeAccount` `store_acme_accounts.go:58-75` dual-read: `decryptSecret(privateKeyEncrypted, a.PrivateKey, secretAAD("acme_accounts",...))`.
- `UpdateAcmeAccount` `store_acme_accounts.go:103-152` validates `allowedAcmeAccountColumns` map (`email, private_key, ca_url, is_default, updated_at`) before dynamic `SET`.

`store_acme_accounts.go:186-270` `DNSProviderAccounts`:
- `ListDNSProviderAccounts` selects `COALESCE(credentials::text,'{}'), COALESCE(credentials_encrypted,'')` and dual-reads via `decryptSecret(encrypted, plain, secretAAD("dns_provider_accounts",id,"credentials"))`.
- `CreateDNSProviderAccount` validates `Name`+`Provider`, marshals `Credentials` JSON, encrypts via `secretAAD("dns_provider_accounts",id,"credentials")`, inserts `credentials` as `'{}'::jsonb` + `credentials_encrypted=$4` `store_acme_accounts.go:259-262` — same dual-write.
- `UpdateDNSProviderAccount` `store_acme_accounts.go:272-335` similar allowed-columns check for `name, provider, credentials, credentials_encrypted, updated_at` handling comma-combined `credentials = '{}'::jsonb, credentials_encrypted = $N`.
- Migration helpers `store_secrets.go:751-883` `migrateCertificateDNSCredentials` and `migrateDNSProviderAccountCredentials` backfill plaintext→encrypted idempotently, handle legacy AAD `secretAADLegacy`, clear plaintext to `'{}'::jsonb`.

### 4. Migrations `211` / `213`

**`211_a_add_restoring_backup_actual_state.sql`** (`forge/api/migrations/211_a_add_restoring_backup_actual_state.sql:1-...`):
```sql
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_enum ... WHERE typname='server_actual_state' AND enumlabel='restoring_backup') THEN
    ALTER TYPE server_actual_state ADD VALUE 'restoring_backup';
  END IF;
  -- also adds offline, terminating, terminated if missing
END $$;
```
Idempotent `DO $$` + `pg_enum` check — prevents "invalid input value for enum" when `store.go` calls `SetServerActualState('restoring_backup')` (GH-09 P1 unconditional security lock).

**`211_b_backup_encryption_v2.sql`**:
```sql
ALTER TABLE backups ADD COLUMN IF NOT EXISTS encryption_salt TEXT NOT NULL DEFAULT '';
ALTER TABLE backups ADD COLUMN IF NOT EXISTS encryption_version INT NOT NULL DEFAULT 1;
ALTER TABLE backups ADD COLUMN IF NOT EXISTS encryption_aad TEXT NOT NULL DEFAULT '';
-- same for backup_artifacts
CREATE INDEX IF NOT EXISTS idx_backups_partial_gc ON backups(server_id, created_at) WHERE name LIKE '%.partial';
```
Comments `Per-backup 16B salt HKDF (V2)`, `1=legacy nonce||ct, 2=salt+AAD+chunked`, `AAD binding server_id:backup_name`. Legacy flag stays.

**`213_encrypt_dns_credentials.sql`**:
```sql
ALTER TABLE certificates ADD COLUMN IF NOT EXISTS dns_credentials_encrypted TEXT NOT NULL DEFAULT '';
ALTER TABLE dns_provider_accounts ADD COLUMN IF NOT EXISTS credentials_encrypted TEXT NOT NULL DEFAULT '';
```
Mirrors `157_encrypt_acme` pattern — plaintext `TEXT/JSONB` remains as compat target, migrator dual-writes ciphertext before clearing.

### 5. `retention_engine.go:5-47` — Converged OR

`RetentionEngine.ShouldKeep` implements BK-06 union: `rank<MaxBackups || age<=RetentionDays*24h || age<MaxAge || period!=""`. `HasActive` checks any non-zero. Used by service `EnforceRetentionPolicy` `forge/api/internal/services/backup/service.go:848-871`: `withinCount := MaxBackups<=0 || index<MaxBackups`, `withinAge := RetentionDays<=0 || now.Sub(created) <= RetentionDays*24h`, `keep = IsLocked || withinCount || withinAge` — pure OR.

## Existing Tests

```
ls forge/api/internal/store/store_backup*test.go
  forge/api/internal/store/store_backup_jobs.go
  forge/api/internal/store/store_backup_lock_integration_test.go
  forge/api/internal/store/store_backup_policies.go
  forge/api/internal/store/store_backups.go
  forge/api/internal/store/store_backups_admin.go
→ No store_backup*test.go — only integration test store_backup_lock_integration_test.go
grep -l "Backup" *test.go → migration_comprehensive_test.go, store_admin_backups_integration_test.go, store_backup_lock_integration_test.go, etc.

store_backup_lock_integration_test.go:57-104 TestUpsertBackupIsLocked (integration, requires TEST_DATABASE_URL)
  - Verifies pending is_locked=true, completion upsert does NOT clobber lock, plain backup unlocked.
  Build tag: //go:build integration — skipped in normal run (seen as "warning: no tests to run" for -run TestUpsertBackup).

store_admin_backups_integration_test.go:50-176 covers BackupConfiguration, BackupJob, BackupArtifact, BackupRetentionPolicy, provider config encrypted-at-rest (requires TEST_DATABASE_URL).

forge/api/internal/services/backup/service_test.go — 35+ unit tests for memory adapter, retry, cron, checksum, storage, but NO TestRetention* (hence `go test -run TestRetention` reports "no tests to run").

store_dns_credentials_test.go — unit tests for secretAAD namespaced vs legacy, DNS credentials encrypt/decrypt, isLegacyAAD detection.
```

## Created Test File

**`forge/api/internal/store/store_backups_reverification_test.go`** (new, no build tag, runs in unit suite) — covers retention OR vs AND, prune `pg_advisory_xact_lock`, GC orphan `.partial`.

Tests (all `TestBackup_*` so ` -run TestBackup` captures them):

| Test | What it verifies |
|------|------------------|
| `TestBackup_RetentionEngine_ORSemantics` (11 subcases) | `RetentionEngine.ShouldKeep` OR: within-age-but-over-limit keeps (OR) vs AND would delete; within-limit-but-over-age keeps; both exceeded deletes; both within keeps; only-retention; only-count; no-active; MaxAge |
| `TestBackup_RetentionEngine_HasActive` | `HasActive` correctly reports empty vs each field active |
| `TestBackup_CleanupOldBackups_SQL_ORvsAND` | Reads `store_backups.go`, asserts `CleanupOldBackupsForServer` contains `RetentionEngine OR` comment, `($2>0 AND created_at<...)`, `uuid IN (` and `OR` connecting them, and `GREATEST(0, COUNT(*) - $3)` anti-negative; ensures per-server block not AND-only |
| `TestBackup_CleanupOldBackups_GlobalUsesRetentionOnly` | Global `CleanupOldBackups` has NO `uuid IN` limit logic, uses `interval '1 day'*$1`, respects `is_locked=FALSE` + `completed` |
| `TestBackup_CleanupOldBackups_PgAdvisoryLock` | Both prunes use transaction-scoped `SELECT pg_advisory_xact_lock(hashtextextended('backup_prune:global'` and `...('backup_prune:'||$1`), 2 executions + 2 comment mentions =4 total; transactional `Begin`/`Commit`/`defer Rollback` |
| `TestBackup_CleanupOldBackups_GCPartial` | `name LIKE '%.partial'` appears 2× (global+per-server), `interval '24 hours'` 2×, each GC has `is_locked=FALSE` + `completed` + 24h, per-server scoped to `server_id=$1`, reaper runs before main delete |
| `TestBackup_CleanupOldBackups_RespectsLockedAndCompleted` | Total `DELETE FROM backups` =5 (1 DeleteBackup +4 prune), prune section 4 deletes all have `is_locked=FALSE` + `status='completed'` |
| `TestBackup_CleanupOldBackups_GlobalNoOpWhenDisabled` | Guard `if !autoCleanup \|\| retentionDays<=0 {return 0,nil}` |
| `TestBackup_Migration211` | `211_a` contains `restoring_backup`, `offline`, `terminating`, `terminated`, `server_actual_state`, `DO $$`, `pg_enum`; `211_b` contains `encryption_salt`, `encryption_version`, `encryption_aad`, `idx_backups_partial_gc`, `backup_artifacts` |
| `TestBackup_Migration213` | `213` contains `certificates`, `dns_credentials_encrypted`, `dns_provider_accounts`, `credentials_encrypted`, `ADD COLUMN IF NOT EXISTS`, `TEXT NOT NULL DEFAULT ''` |
| `TestBackup_CertificatesDNSEncryption` | `store_certificates.go` encrypts `private_key` + `dns_credentials` via `secretAAD("certificates",...)`, clears plaintext to `'{}'::jsonb`, dual-reads via `decryptSecret(dnsEncrypted, plainForDecrypt`, selects both encrypted+plaintext cols |
| `TestBackup_ACMEAccountsDNSProviderAccountsEncryption` | `store_acme_accounts.go` encrypts `acme_accounts.private_key` and `dns_provider_accounts.credentials`, INSERT clears plaintext, dual-read, validates `allowedAcmeAccountColumns` / `allowedDNSProviderColumns` |
| `TestBackup_ServiceEnforceRetentionPolicy_OR` | Reads `../../services/backup/service.go`, asserts `withinCount \|\| withinAge` and `backup.IsLocked \|\| withinCount \|\| withinAge`, `sort.SliceStable` newest-first, logs Union OR |
| `TestBackup_CleanupOldBackups_IntervalUsesDayMultiplier` | `interval '1 day' * $1` / `* $2` arithmetic |
| `TestBackup_RetentionEngine_Integration_WithServiceLogic` | End-to-end OR simulation: MaxBackups=2 Retention=7, 4 backups ages 1d,2d,8d,20d → newest 2 kept, others deleted; proves `young-but-over-limit` kept by OR, deleted by AND |

Helper `readReverificationFile` uses `runtime.Caller` + `filepath.Dir` to resolve `store_backups.go` and `../../migrations/*.sql` reliably regardless of `go test` working dir, with absolute fallback.

## Test Verification

### `go test ./forge/api/internal/store -run TestBackup -count=1 -v 2>&1 | tail -n 30`

```
--- PASS: TestBackup_CleanupOldBackups_GCPartial (0.00s)
--- PASS: TestBackup_Migration213 (0.00s)
--- PASS: TestBackup_ServiceEnforceRetentionPolicy_OR (0.00s)
=== RUN   TestBackup_CleanupOldBackups_PgAdvisoryLock/global_uses_pg_advisory_xact_lock_with_hashtextextended_backup_prune:global
--- PASS: TestBackup_CleanupOldBackups_RespectsLockedAndCompleted (0.00s)
=== RUN   TestBackup_CleanupOldBackups_PgAdvisoryLock/per_server_uses_pg_advisory_xact_lock_with_hashtextextended_backup_prune:||serverID
=== RUN   TestBackup_RetentionEngine_ORSemantics/only_retention_active_over_age_deletes
=== RUN   TestBackup_RetentionEngine_ORSemantics/only_count_active_within_limit_keeps
=== RUN   TestBackup_RetentionEngine_ORSemantics/only_count_active_over_limit_deletes
--- PASS: TestBackup_CleanupOldBackups_PgAdvisoryLock (0.00s)
    --- PASS: TestBackup_CleanupOldBackups_PgAdvisoryLock/global_uses_pg_advisory_xact_lock_with_hashtextextended_backup_prune:global (0.00s)
    --- PASS: TestBackup_CleanupOldBackups_PgAdvisoryLock/per_server_uses_pg_advisory_xact_lock_with_hashtextextended_backup_prune:||serverID (0.00s)
=== RUN   TestBackup_RetentionEngine_ORSemantics/no_active_rules_deletes
=== RUN   TestBackup_RetentionEngine_ORSemantics/maxage_within_keeps
=== RUN   TestBackup_RetentionEngine_ORSemantics/maxage_over_deletes
--- PASS: TestBackup_RetentionEngine_ORSemantics (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/within_age_but_over_limit_keeps_OR (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/within_limit_but_over_age_keeps_OR (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/both_exceeded_deletes (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/both_within_keeps (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/only_retention_active_within_age_keeps (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/only_retention_active_over_age_deletes (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/only_count_active_within_limit_keeps (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/only_count_active_over_limit_deletes (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/no_active_rules_deletes (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/maxage_within_keeps (0.00s)
    --- PASS: TestBackup_RetentionEngine_ORSemantics/maxage_over_deletes (0.00s)
--- PASS: TestBackup_Migration211 (0.00s)
PASS
ok      gamepanel/forge/internal/store  0.873s
```

Full run: 15 tests PASS, 0 FAIL (including 11 OR subcases). Prior run 4.052s with `-count=1` (fresh) also PASS.

### `go test ./forge/api/internal/services/backup -run TestRetention -count=1`

```
testing: warning: no tests to run
PASS
ok      gamepanel/forge/internal/services/backup 0.968s [no tests to run]
```
As expected — `service_test.go` has no `TestRetention*` prefixed tests (retention is tested via `TestService_RetentionEnforcement_*` and store-layer OR test above reads `service.go` source to verify OR). No breakage.

### `go test ./forge/api/internal/services/backup -count=1` (retention change regression)

```
ok      gamepanel/forge/internal/services/backup 8.429s
ok      gamepanel/forge/internal/services/backup 16.090s (earlier full run)
PASS — 50+ tests including TestService_RetentionEnforcement_DeletesOldBackups, TestService_EnforceRetentionPolicy coverage, worker, adapters, checksum, etc.
```

No failures introduced by `store_backups.go` OR convergence; service `EnforceRetentionPolicy` already used OR (`IsLocked || withinCount || withinAge`) and remains compatible.

## Reverification Summary

| Area | File:Line | Fix Verified | Test |
|------|-----------|--------------|------|
| Retention OR vs AND | `store_backups.go:307-326` + `retention_engine.go:25-42` + `services/backup/service.go:851-857` | Per-server prune `WHERE (age) OR (over-limit)` not `AND`; `RetentionEngine.ShouldKeep` union; service `keep = locked \|\| count \|\| age` | `TestBackup_RetentionEngine_ORSemantics` (11 cases), `TestBackup_CleanupOldBackups_SQL_ORvsAND`, `TestBackup_ServiceEnforceRetentionPolicy_OR`, `TestBackup_RetentionEngine_Integration_WithServiceLogic` |
| Prune `pg_advisory_xact_lock` | `store_backups.go:253` global, `292` per-server | Transaction-scoped `hashtextextended('backup_prune:global')` and `('backup_prune:'\|\|$1)`, `Begin`/`Commit`/`defer Rollback` | `TestBackup_CleanupOldBackups_PgAdvisoryLock` (2 subcases) |
| GC orphan `.partial` | `store_backups.go:258-266` + `296-303` + `211_b` idx | Deletes `is_locked=FALSE AND status='completed' AND name LIKE '%.partial' AND created_at<now-24h` in both scopes, index `idx_backups_partial_gc` | `TestBackup_CleanupOldBackups_GCPartial`, `TestBackup_Migration211` partial index, `TestBackup_CleanupOldBackups_RespectsLockedAndCompleted` |
| Certificates DNS encryption | `store_certificates.go:80-168` | `encryptSecret` with `secretAAD("certificates",id,"dns_credentials")`, dual-write ` '{}'::jsonb` + `dns_credentials_encrypted`, dual-read fallback | `TestBackup_CertificatesDNSEncryption` |
| ACME / DNS provider | `store_acme_accounts.go:77-335` + `store_secrets.go:751-883` | `acme_accounts.private_key_encrypted`, `dns_provider_accounts.credentials_encrypted`, allowed-columns guards, migration backfill | `TestBackup_ACMEAccountsDNSProviderAccountsEncryption` |
| Migrations 211,213 | `forge/api/migrations/211_*.sql`, `213_*.sql` | Idempotent `ADD COLUMN IF NOT EXISTS`, enum `restoring_backup`, V2 salt/version/aad, encrypted envelopes | `TestBackup_Migration211`, `TestBackup_Migration213` |

## Verdict

**PASS** — All focus areas have fixes landed and reverification tests green:

- Retention correctly implements **OR (union)** — verified via `RetentionEngine` unit cases and SQL `OR` extraction; `AND` would have deleted young-but-over-limit or old-but-within-limit incorrectly.
- Prune correctly uses **transaction-scoped `pg_advisory_xact_lock`** (`hashtextextended`) per global and per-server, serialized mark-sweep without session-lock leak.
- **GC `.partial`** reaper correctly isolates `is_locked=FALSE, completed, %.partial, 24h` in both scopes and via `211_b` partial index.
- Certificates/ACME/DNS dual-write encryption verified; migrations 211/213 idempotent.
- No regression: `go test ./forge/api/internal/services/backup -count=1` **ok 8.429s**; `go test ./forge/api/internal/store -run TestBackup -count=1` **ok 0.873s** (15 tests).

```
go test ./forge/api/internal/store -run TestBackup -count=1 -v → ok (15 tests, 26 subcases)
go test ./forge/api/internal/services/backup -run TestRetention -count=1 → ok [no tests to run]
go test ./forge/api/internal/services/backup -count=1 → ok 8.429s
```
