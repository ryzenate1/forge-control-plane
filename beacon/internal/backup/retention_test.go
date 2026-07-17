package backup_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/gamepanel/beacon/internal/backup"
)

func TestRetentionPolicy(t *testing.T) {
	// Setup test store
	store := &backup.SQLiteStore{db: setupTestDB(t)}

	// Create test backups
	now := time.Now()
	backups := []backup.Backup{
		{ID: "1", ServerID: "server-1", CompletedAt: now.Add(-24 * time.Hour), Status: backup.BackupStatusCompleted},
		{ID: "2", ServerID: "server-1", CompletedAt: now.Add(-48 * time.Hour), Status: backup.BackupStatusCompleted},
		{ID: "3", ServerID: "server-1", CompletedAt: now.Add(-72 * time.Hour), Status: backup.BackupStatusCompleted},
	}

	for _, b := range backups {
		err := store.Create(context.Background(), b)
		require.NoError(t, err)
	}

	// Test retention policy
	policy := backup.RetentionPolicy{
		MaxBackups:  2,
		MaxAge:      48 * time.Hour,
		KeepDaily:   1,
		KeepWeekly:  0,
		KeepMonthly: 0,
	}

	err := policy.Apply(context.Background(), store, "server-1")
	assert.NoError(t, err)

	// Verify only the most recent backup remains
	remaining, err := store.List(context.Background(), "server-1", 0)
	assert.NoError(t, err)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "1", remaining[0].ID)
}
