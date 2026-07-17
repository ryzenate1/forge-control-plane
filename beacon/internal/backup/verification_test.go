package backup_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/gamepanel/beacon/internal/backup"
)

func TestVerifyBackup(t *testing.T) {
	// Setup test store and adapter
	store := &backup.SQLiteStore{db: setupTestDB(t)}
	adapter := &backup.LocalAdapter{}

	// Create test backup
	testBackup := backup.Backup{
		ID:          "test-backup",
		ServerID:    "server-1",
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(time.Minute),
		Status:      backup.BackupStatusCompleted,
		SizeBytes:   1024,
		Files:       5,
		Duration:    time.Minute,
		Adapter:     "local",
		Path:        "/backups/test",
	}

	err := store.Create(context.Background(), testBackup)
	require.NoError(t, err)

	// Test verification
	result, err := backup.VerifyBackup(context.Background(), store, adapter, "test-backup")
	assert.NoError(t, err)
	assert.Equal(t, backup.VerificationStatusCompleted, result.Status)
	assert.Equal(t, "test-backup", result.BackupID)
	assert.NotEmpty(t, result.Checksum)

	// Verify the verification was recorded in the store
	updatedBackup, err := store.Get(context.Background(), "test-backup")
	assert.NoError(t, err)
	assert.Equal(t, backup.BackupStatusCompleted, updatedBackup.Status)
}
