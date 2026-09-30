//go:build integration

package store

import (
	"context"
	"testing"
)

// TestIsServerRestoreBlocking covers the unconditional restoring lock (GH-09 P1).
// Verifies:
// - freshly created server is not restoring
// - after setting actual_state to restoring_backup, IsServerRestoreBlocking returns true
// - concurrent power/install would be blocked (409) while restoring
// - after clearing actual_state to stopped, lock is released
// - backup row with status='restoring' also triggers the lock
// - restored/failed status releases lock (unless actual_state still restoring)
func TestIsServerRestoreBlocking(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	serverID := seedBackupServer(t, s)

	// Initially not restoring
	blocked, err := s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("initial IsServerRestoreBlocking: %v", err)
	}
	if blocked {
		t.Fatal("expected not blocked initially")
	}

	// Set actual_state to restoring_backup -> should block
	if err := s.SetServerActualState(ctx, serverID, ServerActualStateRestoringBackup, "test restore start"); err != nil {
		t.Fatalf("SetServerActualState restoring: %v", err)
	}
	blocked, err = s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("after restoring IsServerRestoreBlocking: %v", err)
	}
	if !blocked {
		t.Fatal("expected blocked when actual_state=restoring_backup")
	}
	// Verify alias works
	blockedAlias, err := s.IsServerRestoring(ctx, serverID)
	if err != nil || !blockedAlias {
		t.Fatalf("IsServerRestoring alias should match: %v blocked=%v", err, blockedAlias)
	}

	// Clear to stopped -> should not block if no backup restoring row
	if err := s.SetServerActualState(ctx, serverID, ServerActualStateStopped, "test restore done"); err != nil {
		t.Fatalf("SetServerActualState stopped: %v", err)
	}
	blocked, err = s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("after stopped IsServerRestoreBlocking: %v", err)
	}
	if blocked {
		t.Fatal("expected not blocked after clearing actual_state, no backup restoring row")
	}

	// Create a backup row with restoring status -> should block even though actual_state is stopped
	_, err = s.UpsertBackup(ctx, serverID, UpsertBackupRequest{Name: "restore-lock-test", Status: "restoring"}, nil)
	if err != nil {
		t.Fatalf("UpsertBackup restoring: %v", err)
	}
	blocked, err = s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("after backup restoring IsServerRestoreBlocking: %v", err)
	}
	if !blocked {
		t.Fatal("expected blocked when backup status=restoring")
	}
	// Mark restored -> should release (actual_state already stopped)
	if err := s.MarkBackupStatus(ctx, serverID, "restore-lock-test", "restored", nil); err != nil {
		t.Fatalf("MarkBackupStatus restored: %v", err)
	}
	blocked, err = s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("after restored IsServerRestoreBlocking: %v", err)
	}
	if blocked {
		t.Fatal("expected not blocked after backup restored")
	}
	// Ensure failure also releases when actual_state cleared
	_, err = s.UpsertBackup(ctx, serverID, UpsertBackupRequest{Name: "restore-lock-test-2", Status: "restoring"}, nil)
	if err != nil {
		t.Fatalf("UpsertBackup restoring 2: %v", err)
	}
	if err := s.SetServerActualState(ctx, serverID, ServerActualStateRestoringBackup, "restore again"); err != nil {
		t.Fatalf("SetServerActualState restoring 2: %v", err)
	}
	blocked, _ = s.IsServerRestoreBlocking(ctx, serverID)
	if !blocked {
		t.Fatal("expected blocked with both locks")
	}
	if err := s.MarkBackupStatus(ctx, serverID, "restore-lock-test-2", "restore_failed", nil); err != nil {
		t.Fatalf("MarkBackupStatus failed: %v", err)
	}
	// Still blocked via actual_state
	blocked, _ = s.IsServerRestoreBlocking(ctx, serverID)
	if !blocked {
		t.Fatal("expected still blocked via actual_state after backup failed")
	}
	if err := s.SetServerActualState(ctx, serverID, ServerActualStateStopped, "restore failed clear"); err != nil {
		t.Fatalf("clear actual_state: %v", err)
	}
	blocked, _ = s.IsServerRestoreBlocking(ctx, serverID)
	if blocked {
		t.Fatal("expected not blocked after clearing both")
	}
}

// TestConcurrentRestoreAndPowerRace simulates the race that GH-09 reports:
// a power start dispatched concurrently with a restore must be rejected with 409.
// At the store layer we verify the lock is visible immediately after SetServerActualState.
func TestConcurrentRestoreAndPowerRace(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	serverID := seedBackupServer(t, s)

	// Simulate restore start: set actual_state to restoring_backup (what the HTTP handler does before Dispatch)
	if err := s.SetServerActualState(ctx, serverID, ServerActualStateRestoringBackup, "backup restore queued"); err != nil {
		t.Fatalf("SetServerActualState: %v", err)
	}
	// Concurrent power check (ensureRestoreIdle path) must now see blocking
	blocked, err := s.IsServerRestoreBlocking(ctx, serverID)
	if err != nil {
		t.Fatalf("IsServerRestoreBlocking: %v", err)
	}
	if !blocked {
		t.Fatal("concurrent power should be blocked (409) while restore is queued")
	}
	// Simulate concurrent second restore also blocked
	blocked2, _ := s.IsServerRestoreBlocking(ctx, serverID)
	if !blocked2 {
		t.Fatal("concurrent second restore should be blocked")
	}
	// Transfer should be independent but restore should not affect transfer blocking (separate)
	transferBlocked, _ := s.IsServerTransferBlocking(ctx, serverID)
	if transferBlocked {
		t.Fatal("transfer should not be blocked by restore alone")
	}
	// Cleanup
	_ = s.SetServerActualState(ctx, serverID, ServerActualStateStopped, "test done")
	_, _ = s.db.Exec(ctx, `DELETE FROM backups WHERE server_id=$1`, serverID)
}
