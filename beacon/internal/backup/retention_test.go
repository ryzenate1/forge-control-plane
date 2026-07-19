package backup_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gamepanel/beacon/internal/backup"
)

func TestRetentionPolicy(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
        CREATE TABLE backups (
            id TEXT PRIMARY KEY,
            server_id TEXT,
            started_at DATETIME,
            completed_at DATETIME,
            status TEXT,
            size_bytes INTEGER,
            files INTEGER,
            duration INTEGER,
            adapter TEXT,
            path TEXT,
            error TEXT
        );
    `)
	require.NoError(t, err)

	store := backup.NewSQLiteStore(db)

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

	policy := backup.RetentionPolicy{
		MaxBackups:  2,
		MaxAge:      48 * time.Hour,
		KeepDaily:   1,
		KeepWeekly:  0,
		KeepMonthly: 0,
	}

	err = policy.Apply(context.Background(), store, "server-1")
	assert.NoError(t, err)

	remaining, err := store.List(context.Background(), "server-1", 0)
	assert.NoError(t, err)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "1", remaining[0].ID)
}
