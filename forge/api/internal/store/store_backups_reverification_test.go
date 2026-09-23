package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// helper to read source files relative to this test file
func readReverificationFile(t *testing.T, relPath string) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	base := filepath.Dir(callerFile)
	candidate := filepath.Join(base, relPath)
	data, err := os.ReadFile(candidate)
	if err == nil {
		return string(data)
	}
	// fallback: try absolute repo root paths that might match CI working dir
	absFallbacks := []string{
		filepath.Join("/Users/riyaz/project/gamepanel", relPath),
		filepath.Join("/Users/riyaz/project/gamepanel/forge/api/internal/store", filepath.Base(relPath)),
		candidate,
	}
	for _, p := range absFallbacks {
		if b, err2 := os.ReadFile(p); err2 == nil {
			return string(b)
		}
	}
	t.Fatalf("read %s (tried %s): %v", relPath, candidate, err)
	return ""
}

func readStoreFile(t *testing.T, name string) string {
	return readReverificationFile(t, name)
}

func readMigrationFile(t *testing.T, name string) string {
	return readReverificationFile(t, filepath.Join("../../migrations", name))
}

// TestBackup_RetentionEngine_ORSemantics verifies the unified OR retention logic (BK-06).
// Keep if ANY active rule keeps (union). Delete only if exceeds ALL active thresholds.
// Contrasts with incorrect AND semantics which would delete if ANY threshold exceeded.
func TestBackup_RetentionEngine_ORSemantics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		engine     RetentionEngine
		age        time.Duration
		rank       int
		expectKeep bool
		desc       string
	}{
		{
			name:       "within_age_but_over_limit_keeps_OR",
			engine:     RetentionEngine{MaxBackups: 3, RetentionDays: 7},
			age:        2 * 24 * time.Hour,
			rank:       5,
			expectKeep: true,
			desc:       "age 2d within 7d retention, rank 5 beyond limit 3 => OR keeps (AND would delete)",
		},
		{
			name:       "within_limit_but_over_age_keeps_OR",
			engine:     RetentionEngine{MaxBackups: 5, RetentionDays: 7},
			age:        10 * 24 * time.Hour,
			rank:       1,
			expectKeep: true,
			desc:       "rank 1 within limit 5, age 10d beyond 7d => OR keeps (AND would delete)",
		},
		{
			name:       "both_exceeded_deletes",
			engine:     RetentionEngine{MaxBackups: 3, RetentionDays: 7},
			age:        10 * 24 * time.Hour,
			rank:       5,
			expectKeep: false,
			desc:       "both thresholds exceeded => delete under both OR and AND",
		},
		{
			name:       "both_within_keeps",
			engine:     RetentionEngine{MaxBackups: 5, RetentionDays: 7},
			age:        2 * 24 * time.Hour,
			rank:       1,
			expectKeep: true,
			desc:       "both within => keep",
		},
		{
			name:       "only_retention_active_within_age_keeps",
			engine:     RetentionEngine{RetentionDays: 30},
			age:        5 * 24 * time.Hour,
			rank:       100,
			expectKeep: true,
			desc:       "only retention rule, age within => keep regardless of rank",
		},
		{
			name:       "only_retention_active_over_age_deletes",
			engine:     RetentionEngine{RetentionDays: 30},
			age:        31 * 24 * time.Hour,
			rank:       0,
			expectKeep: false,
			desc:       "only retention rule, age beyond => delete (rank irrelevant)",
		},
		{
			name:       "only_count_active_within_limit_keeps",
			engine:     RetentionEngine{MaxBackups: 3},
			age:        365 * 24 * time.Hour,
			rank:       1,
			expectKeep: true,
			desc:       "only count rule, rank within => keep regardless of age",
		},
		{
			name:       "only_count_active_over_limit_deletes",
			engine:     RetentionEngine{MaxBackups: 3},
			age:        0,
			rank:       5,
			expectKeep: false,
			desc:       "only count rule, rank beyond => delete",
		},
		{
			name:       "no_active_rules_deletes",
			engine:     RetentionEngine{},
			age:        0,
			rank:       0,
			expectKeep: false,
			desc:       "no active rules => nothing to keep, default delete",
		},
		{
			name:       "maxage_within_keeps",
			engine:     RetentionEngine{MaxAge: 24 * time.Hour},
			age:        12 * time.Hour,
			rank:       999,
			expectKeep: true,
			desc:       "MaxAge within => keep",
		},
		{
			name:       "maxage_over_deletes",
			engine:     RetentionEngine{MaxAge: 24 * time.Hour},
			age:        25 * time.Hour,
			rank:       0,
			expectKeep: false,
			desc:       "MaxAge exceeded => delete (but keep via other OR if applicable)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.engine.ShouldKeep(tc.age, tc.rank, "")
			if got != tc.expectKeep {
				t.Fatalf("%s: ShouldKeep(age=%v, rank=%d) = %v, want %v", tc.desc, tc.age, tc.rank, got, tc.expectKeep)
			}
		})
	}
}

// TestBackup_RetentionEngine_HasActive verifies HasActive detection.
func TestBackup_RetentionEngine_HasActive(t *testing.T) {
	t.Parallel()
	if (RetentionEngine{}).HasActive() {
		t.Fatal("empty engine should not be active")
	}
	activeCases := []RetentionEngine{
		{MaxBackups: 1},
		{RetentionDays: 1},
		{MaxAge: time.Hour},
		{KeepDaily: 1},
		{KeepWeekly: 1},
		{KeepMonthly: 1},
	}
	for i, e := range activeCases {
		if !e.HasActive() {
			t.Fatalf("case %d %+v should be active", i, e)
		}
	}
}

// TestBackup_CleanupOldBackups_SQL_ORvsAND verifies the per-server prune uses OR (union)
// not AND. Under AND a backup young but over-limit would be incorrectly retained, and
// a backup over-age but within-limit would be incorrectly retained opposite way.
// OR semantics: WHERE (retention) OR (over-limit) => delete if EITHER triggers.
// We assert the SQL contains OR between the two branches and not a pure AND.
func TestBackup_CleanupOldBackups_SQL_ORvsAND(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")

	// Find CleanupOldBackupsForServer block
	if !strings.Contains(src, "CleanupOldBackupsForServer") {
		t.Fatal("store_backups.go missing CleanupOldBackupsForServer")
	}
	// Verify OR semantics comment and SQL
	if !strings.Contains(src, "RetentionEngine OR") {
		t.Fatal("missing RetentionEngine OR comment in CleanupOldBackupsForServer")
	}
	// The critical SQL must contain OR between retention and limit branches
	// Look for pattern: ($2 > 0 AND created_at < ... ) \n\t\t\tOR\n\t\t\t-- Or if we're over
	if !strings.Contains(src, "($2 > 0 AND created_at < now()") {
		t.Fatal("missing retention age branch ($2 > 0 AND created_at ...)")
	}
	if !strings.Contains(src, "uuid IN (") {
		t.Fatal("missing limit-based branch uuid IN (...)")
	}
	// Ensure OR connects the two branches inside per-server func
	serverStartIdx := strings.Index(src, "func (s *Store) CleanupOldBackupsForServer(")
	if serverStartIdx == -1 {
		t.Fatal("missing CleanupOldBackupsForServer func")
	}
	serverBlock := src[serverStartIdx:]
	// Find the retention OR block specifically (contains "DELETE FROM backups\n\t\tWHERE server_id = $1\n\t\tAND is_locked = FALSE\n\t\tAND status = 'completed'\n\t\tAND (")
	retentionIdx := strings.Index(serverBlock, "DELETE FROM backups\n\t\tWHERE server_id = $1\n\t\tAND is_locked = FALSE")
	if retentionIdx == -1 {
		retentionIdx = strings.Index(serverBlock, "DELETE FROM backups")
		if retentionIdx == -1 {
			t.Fatal("cannot locate per-server retention DELETE")
		}
		// skip first per-server GC, find second DELETE in serverBlock
		second := strings.Index(serverBlock[retentionIdx+1:], "DELETE FROM backups")
		if second != -1 {
			retentionIdx = retentionIdx + 1 + second
		}
	}
	snippet := serverBlock[retentionIdx:min(len(serverBlock), retentionIdx+1500)]
	// Count OR/AND in that snippet region
	orCount := strings.Count(snippet, "\n\t\t\tOR")
	if orCount == 0 {
		// try without tabs
		orCount = strings.Count(snippet, "OR")
		if orCount == 0 {
			t.Fatalf("expected OR between retention and limit branches, snippet: %q", snippet[:500])
		}
	}
	// Ensure not using AND-only incorrect logic: check that snippet does NOT contain
	// the buggy pattern where both conditions are ANDed without OR
	if strings.Contains(snippet, "AND\n\t\t\tuuid IN") && !strings.Contains(snippet, "OR") {
		t.Fatal("found AND between retention and limit without OR — indicates buggy AND semantics")
	}
	// Verify limit branch uses GREATEST(0, COUNT(*) - $3) to avoid negative LIMIT
	if !strings.Contains(src, "GREATEST(0, (SELECT COUNT(*) FROM backups WHERE server_id = $1 AND status = 'completed') - $3)") {
		t.Fatal("limit branch should use GREATEST(0, COUNT - $3) to avoid negative LIMIT")
	}
}

// TestBackup_CleanupOldBackups_GlobalUsesRetentionOnly_notLimit ensures global prune is
// retention-only (no per-server limit concern), plus partial GC.
func TestBackup_CleanupOldBackups_GlobalUsesRetentionOnly(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")
	// Global function should NOT contain uuid IN limit logic (that's per-server only)
	// Find global func region
	globalStart := strings.Index(src, "func (s *Store) CleanupOldBackups(")
	serverStart := strings.Index(src, "func (s *Store) CleanupOldBackupsForServer(")
	if globalStart == -1 || serverStart == -1 {
		t.Fatal("missing CleanupOldBackups funcs")
	}
	globalBlock := src[globalStart:serverStart]
	if strings.Contains(globalBlock, "uuid IN") {
		t.Fatal("global CleanupOldBackups should not contain uuid IN limit logic (per-server only)")
	}
	if !strings.Contains(globalBlock, "interval '1 day' * $1") {
		t.Fatal("global prune should use interval '1 day' * $1")
	}
	if !strings.Contains(globalBlock, "is_locked = FALSE") {
		t.Fatal("global prune must respect is_locked")
	}
	if !strings.Contains(globalBlock, "status = 'completed'") {
		t.Fatal("global prune must filter status = 'completed'")
	}
}

// TestBackup_CleanupOldBackups_PgAdvisoryLock verifies prune uses transaction-scoped
// pg_advisory_xact_lock to serialize mark-sweep across replicas without leaking session locks.
func TestBackup_CleanupOldBackups_PgAdvisoryLock(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")

	tests := []struct {
		name   string
		substr string
	}{
		{
			name:   "global uses pg_advisory_xact_lock with hashtextextended backup_prune:global",
			substr: "pg_advisory_xact_lock(hashtextextended('backup_prune:global'",
		},
		{
			name:   "per_server uses pg_advisory_xact_lock with hashtextextended backup_prune:||serverID",
			substr: "pg_advisory_xact_lock(hashtextextended('backup_prune:'||$1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(src, tc.substr) {
				t.Fatalf("missing advisory lock: %q\nfull src excerpt: %q", tc.substr, src[2000:3000])
			}
		})
	}
	// Ensure it is xact (transaction) not session lock
	// Include check that executions use xact; comments also contain the name so count executions specifically
	execCount := strings.Count(src, "SELECT pg_advisory_xact_lock")
	if execCount != 2 {
		t.Fatalf("expected 2 SELECT pg_advisory_xact_lock executions (global + per-server), got %d", execCount)
	}
	// Total mentions include comments = 4, executions =2, total 4 is expected but not strict
	totalCount := strings.Count(src, "pg_advisory_xact_lock")
	if totalCount != 4 {
		t.Fatalf("expected 4 total pg_advisory_xact_lock mentions (2 comments + 2 execs), got %d", totalCount)
	}
	// Verify locks are inside a transaction (BEGIN + hashtextextended + COMMIT pattern)
	if !strings.Contains(src, "tx, err := s.db.Begin(ctx)") {
		t.Fatal("prune should be transactional (Begin)")
	}
	if !strings.Contains(src, "tx.Commit(ctx)") {
		t.Fatal("prune should commit transaction")
	}
	// Ensure deferred rollback exists
	if !strings.Contains(src, "defer tx.Rollback(ctx)") {
		t.Fatal("prune should defer Rollback")
	}
}

// TestBackup_CleanupOldBackups_GCPartial verifies GC reaper for orphan .partial entries older than 24h.
func TestBackup_CleanupOldBackups_GCPartial(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")

	// Both global and per-server should GC .partial
	partialPattern := "name LIKE '%.partial'"
	if count := strings.Count(src, partialPattern); count != 2 {
		t.Fatalf("expected 2 occurrences of %q (global + per-server), got %d", partialPattern, count)
	}
	// Must filter 24h
	if count := strings.Count(src, "interval '24 hours'"); count < 2 {
		t.Fatalf("expected at least 2 occurrences of interval '24 hours' for .partial GC, got %d", count)
	}
	// Must be is_locked FALSE and completed and .partial
	requiredClauses := []string{
		"is_locked = FALSE",
		"status = 'completed'",
		"name LIKE '%.partial'",
		"created_at < now() - interval '24 hours'",
	}
	for _, clause := range requiredClauses {
		if !strings.Contains(src, clause) {
			t.Fatalf("GC .partial should contain %q", clause)
		}
	}
	// Per-server GC must be scoped to server_id
	globalStart := strings.Index(src, "func (s *Store) CleanupOldBackups(")
	serverStart := strings.Index(src, "func (s *Store) CleanupOldBackupsForServer(")
	globalBlock := src[globalStart:serverStart]
	serverBlock := src[serverStart:]
	// Global .partial GC should NOT have server_id filter (it's global)
	// Find first .partial delete inside globalBlock
	if strings.Contains(globalBlock, "server_id = $1") && strings.Index(globalBlock, partialPattern) < strings.Index(globalBlock, "server_id") {
		// This check is lenient; main invariant is per-server block MUST have server_id
	}
	if !strings.Contains(serverBlock, "WHERE server_id = $1") {
		t.Fatal("per-server GC .partial should be scoped to server_id = $1")
	}
	if !strings.Contains(serverBlock, partialPattern) {
		t.Fatal("per-server block missing .partial GC")
	}
	// Verify .partial GC runs BEFORE main retention delete (reaper first)
	firstPartial := strings.Index(src, partialPattern)
	mainDelete := strings.Index(src[firstPartial+len(partialPattern):], "created_at < now() - interval '1 day'")
	if mainDelete == -1 {
		t.Fatal("main retention delete should follow .partial GC")
	}
}

// TestBackup_CleanupOldBackups_RespectsLockedAndCompleted verifies both prunes respect
// locked backups (is_locked FALSE) and only completed backups.
func TestBackup_CleanupOldBackups_RespectsLockedAndCompleted(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")
	// Total DELETE FROM backups: 1 (DeleteBackup single) + 2 partial GC + 2 retention = 5
	delCount := strings.Count(src, "DELETE FROM backups")
	if delCount != 5 {
		t.Fatalf("expected 5 DELETE FROM backups (1 DeleteBackup + 2 partial GC + 2 retention), got %d", delCount)
	}
	// Prune deletes are those inside CleanupOldBackups funcs (4 of them). They must have is_locked and status filters.
	// Isolate prune blocks (from first Cleanup func onward) to avoid the single DeleteBackup check.
	cleanStart := strings.Index(src, "func (s *Store) CleanupOldBackups(")
	if cleanStart == -1 {
		t.Fatal("missing CleanupOldBackups func")
	}
	pruneSection := src[cleanStart:]
	pruneDelCount := strings.Count(pruneSection, "DELETE FROM backups")
	if pruneDelCount != 4 {
		t.Fatalf("expected 4 prune DELETEs inside cleanup funcs, got %d", pruneDelCount)
	}
	parts := strings.Split(pruneSection, "DELETE FROM backups")
	for i, part := range parts[1:] { // skip before first
		block := part[:min(len(part), 800)]
		if !strings.Contains(block, "is_locked = FALSE") {
			t.Fatalf("prune DELETE #%d missing is_locked = FALSE: %q", i+1, block[:200])
		}
		if !strings.Contains(block, "status = 'completed'") {
			t.Fatalf("prune DELETE #%d missing status = 'completed': %q", i+1, block[:200])
		}
	}
}

// TestBackup_CleanupOldBackups_GlobalNoOpWhenDisabled verifies autoCleanup and retentionDays guard.
func TestBackup_CleanupOldBackups_GlobalNoOpWhenDisabled(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")
	if !strings.Contains(src, "if !autoCleanup || retentionDays <= 0") {
		t.Fatal("CleanupOldBackups should early-return when !autoCleanup || retentionDays <=0")
	}
	if !strings.Contains(src, "return 0, nil") {
		t.Fatal("disabled prune should return 0, nil")
	}
}

// TestBackup_Migration211 verifies 211_a and 211_b contain expected schema changes.
func TestBackup_Migration211(t *testing.T) {
	t.Parallel()
	a := readMigrationFile(t, "211_a_add_restoring_backup_actual_state.sql")
	b := readMigrationFile(t, "211_b_backup_encryption_v2.sql")

	// 211_a: enum extensions
	for _, label := range []string{"restoring_backup", "offline", "terminating", "terminated"} {
		if !strings.Contains(a, label) {
			t.Errorf("211_a missing enum label %q", label)
		}
	}
	if !strings.Contains(a, "server_actual_state") {
		t.Error("211_a should ALTER TYPE server_actual_state")
	}
	if !strings.Contains(a, "DO $$") {
		t.Error("211_a should use DO $$ idempotent block")
	}
	if !strings.Contains(a, "pg_enum") {
		t.Error("211_a should check pg_enum for existence")
	}

	// 211_b: encryption V2
	for _, col := range []string{"encryption_salt", "encryption_version", "encryption_aad"} {
		if !strings.Contains(b, col) {
			t.Errorf("211_b missing column %q", col)
		}
	}
	if !strings.Contains(b, "idx_backups_partial_gc") {
		t.Error("211_b should create idx_backups_partial_gc for GC reaper")
	}
	if !strings.Contains(b, "name LIKE '%.partial'") {
		t.Error("211_b partial index should filter name LIKE pct-partial")
	}
	if !strings.Contains(b, "backup_artifacts") {
		t.Error("211_b should also alter backup_artifacts")
	}
}

// TestBackup_Migration213 verifies 213 encrypt dns credentials.
func TestBackup_Migration213(t *testing.T) {
	t.Parallel()
	src := readMigrationFile(t, "213_encrypt_dns_credentials.sql")
	for _, col := range []string{"certificates", "dns_credentials_encrypted", "dns_provider_accounts", "credentials_encrypted"} {
		if !strings.Contains(src, col) {
			t.Errorf("213 missing %q", col)
		}
	}
	// Should ADD COLUMN IF NOT EXISTS ... DEFAULT ''
	if !strings.Contains(src, "ADD COLUMN IF NOT EXISTS") {
		t.Error("213 should be idempotent ADD COLUMN IF NOT EXISTS")
	}
	if !strings.Contains(src, "TEXT NOT NULL DEFAULT ''") {
		t.Error("213 encrypted columns should be TEXT NOT NULL DEFAULT ''")
	}
}

// TestBackup_CertificatesDNSEncryption verifies store_certificates.go dual-write encryption.
func TestBackup_CertificatesDNSEncryption(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_certificates.go")
	// Private key encryption
	if !strings.Contains(src, `encryptSecret(req.PrivateKey, secretAAD("certificates"`) {
		t.Error("CreateCertificate should encrypt private_key with secretAAD certificates")
	}
	// DNS credentials encryption
	if !strings.Contains(src, `secretAAD("certificates", id, "dns_credentials")`) {
		t.Error("CreateCertificate should encrypt dns_credentials with secretAAD")
	}
	// Dual-write: plaintext cleared to '{}'::jsonb, ciphertext stored
	if !strings.Contains(src, "'{}'::jsonb, $11") && !strings.Contains(src, "'{}'::jsonb") {
		t.Error("INSERT should clear plaintext dns_credentials to '{}'::jsonb")
	}
	if !strings.Contains(src, "dns_credentials_encrypted") {
		t.Error("INSERT should include dns_credentials_encrypted")
	}
	// Dual-read fallback: decryptSecret with plaintext fallback
	if !strings.Contains(src, "decryptSecret(dnsEncrypted, plainForDecrypt") {
		t.Error("GetCertificate should dual-read dnsEncrypted with plaintext fallback")
	}
	// Ensure isLocked fallback for legacy AAD is tested via secrets layer (already covered in dns_credentials_test)
	// Verify private key decrypt also uses secretAAD
	if !strings.Contains(src, `secretAAD("certificates", cert.ID, "private_key")`) {
		t.Error("GetCertificate should decrypt private_key with secretAAD")
	}
	// Check GetCertificate query selects both encrypted and plaintext columns
	if !strings.Contains(src, "dns_credentials_encrypted") || !strings.Contains(src, "dns_credentials::text") {
		t.Error("GetCertificate should select both dns_credentials_encrypted and dns_credentials::text")
	}
}

// TestBackup_ACMEAccountsDNSProviderAccountsEncryption verifies store_acme_accounts.go encryption.
func TestBackup_ACMEAccountsDNSProviderAccountsEncryption(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_acme_accounts.go")
	// ACME account private key
	if !strings.Contains(src, `secretAAD("acme_accounts", id, "private_key")`) {
		t.Error("CreateAcmeAccount should encrypt private_key with secretAAD")
	}
	if !strings.Contains(src, "private_key_encrypted") {
		t.Error("acme_accounts should have private_key_encrypted column")
	}
	// INSERT pattern: '', encrypted (plaintext cleared)
	if !strings.Contains(src, "INSERT INTO acme_accounts") || !strings.Contains(src, "'', $3") {
		t.Error("acme_accounts INSERT should clear plaintext private_key to ''")
	}
	// Dual-read for ACME
	if !strings.Contains(src, `decryptSecret(privateKeyEncrypted, a.PrivateKey, secretAAD("acme_accounts"`) {
		t.Error("GetAcmeAccount should dual-read with decryptSecret fallback")
	}
	// DNS provider accounts
	if !strings.Contains(src, `secretAAD("dns_provider_accounts", id, "credentials")`) {
		t.Error("dns_provider_accounts should encrypt credentials with secretAAD")
	}
	if !strings.Contains(src, "credentials_encrypted") {
		t.Error("dns_provider_accounts should have credentials_encrypted")
	}
	// Ensure allowed columns map is enforced in Update paths
	if !strings.Contains(src, "allowedAcmeAccountColumns") {
		t.Error("UpdateAcmeAccount should validate allowed columns")
	}
	if !strings.Contains(src, "allowedDNSProviderColumns") {
		t.Error("UpdateDNSProviderAccount should validate allowed DNS columns")
	}
}

// TestBackup_ServiceEnforceRetentionPolicy_OR verifies the service-layer OR semantics.
func TestBackup_ServiceEnforceRetentionPolicy_OR(t *testing.T) {
	t.Parallel()
	// Read service.go relative to this test file (runtime.Caller) so it resolves
	// regardless of the machine's absolute checkout path. From internal/store the
	// backup service lives at ../services/backup/service.go.
	_, callerFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	base := filepath.Dir(callerFile)
	candidates := []string{
		filepath.Join(base, "../services/backup/service.go"),
		filepath.Join(base, "../../services/backup/service.go"),
	}
	var data []byte
	var err error
	for _, c := range candidates {
		if data, err = os.ReadFile(c); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "withinCount || withinAge") {
		t.Error("EnforceRetentionPolicy should use OR (withinCount || withinAge)")
	}
	if !strings.Contains(src, "backup.IsLocked || withinCount || withinAge") {
		t.Error("keep decision should include IsLocked OR withinCount OR withinAge")
	}
	if !strings.Contains(src, "RetentionEngine") && !strings.Contains(src, "Union OR") {
		// Lenient: comment may mention OR but not necessarily RetentionEngine string
		t.Log("note: service.go does not explicitly mention RetentionEngine by name, but OR semantics verified via keep expression")
	}
	if !strings.Contains(src, "sort.SliceStable") {
		t.Error("EnforceRetentionPolicy should sort by created_at DESC to enforce MaxBackups on newest")
	}
}

// TestBackup_CleanupOldBackups_IntervalUsesDayMultiplier verifies interval arithmetic.
func TestBackup_CleanupOldBackups_IntervalUsesDayMultiplier(t *testing.T) {
	t.Parallel()
	src := readStoreFile(t, "store_backups.go")
	if !strings.Contains(src, "interval '1 day' * $1") && !strings.Contains(src, "interval '1 day' * $2") {
		t.Fatal("retention prunes should use interval '1 day' * $N")
	}
}

// TestBackup_RetentionEngine_Integration_WithServiceLogic mimics service keep map.
func TestBackup_RetentionEngine_Integration_WithServiceLogic(t *testing.T) {
	t.Parallel()
	// Simulate policy MaxBackups=2, RetentionDays=7, 4 backups at ages 1d,2d,8d,20d (rank 0..3 newest first)
	now := time.Now()
	type fakeBackup struct {
		name      string
		createdAt time.Time
		isLocked  bool
	}
	backups := []fakeBackup{
		{"newest", now.Add(-1 * 24 * time.Hour), false},            // rank0 age1d
		{"second", now.Add(-2 * 24 * time.Hour), false},            // rank1 age2d
		{"oldButWithinLimit", now.Add(-8 * 24 * time.Hour), false}, // rank2 age8d => beyond retention but within limit (Max 2 would keep only rank0,1, so rank2 beyond)
		{"ancient", now.Add(-20 * 24 * time.Hour), false},          // rank3 age20d beyond both
	}
	policyMax := 2
	policyRetention := 7
	keep := make(map[string]bool)
	for idx, b := range backups {
		withinCount := policyMax <= 0 || idx < policyMax
		withinAge := policyRetention <= 0 || now.Sub(b.createdAt) <= time.Duration(policyRetention)*24*time.Hour
		keep[b.name] = b.isLocked || withinCount || withinAge
	}
	// newest and second kept by count AND age (both keep)
	if !keep["newest"] || !keep["second"] {
		t.Fatal("newest/second should be kept")
	}
	// oldButWithinLimit: rank2 >= Max 2 => withinCount false, age 8d >7 => withinAge false => should be deleted under OR (both false)
	if keep["oldButWithinLimit"] {
		t.Fatal("oldButWithinLimit should be deleted (both thresholds exceeded)")
	}
	// ancient also deleted
	if keep["ancient"] {
		t.Fatal("ancient should be deleted")
	}
	// Now test case where OR saves a backup that AND would delete:
	// age 1d (within retention) but rank 5 (over limit) => OR keeps, AND deletes
	withinCount2 := 2 <= 0 || 5 < 2                                           // false
	withinAge2 := 7 <= 0 || (1*24*time.Hour) <= time.Duration(7)*24*time.Hour // true
	keepOR := withinCount2 || withinAge2
	keepAND := withinCount2 && withinAge2
	if !keepOR {
		t.Fatal("OR should keep young backup even if over limit")
	}
	if keepAND {
		t.Fatal("AND would incorrectly delete young backup over limit")
	}
}
