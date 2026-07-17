package backup_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/gamepanel/beacon/internal/backup"
)

func TestSQLiteStore(t *testing.T) {
	// Setup test database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create tables
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

	store := &backup.SQLiteStore{db: db}

	// Test Create and Get
	testBackup := backup.Backup{
		ID:          "test-id",
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

	err = store.Create(context.Background(), testBackup)
	assert.NoError(t, err)

	retrieved, err := store.Get(context.Background(), "test-id")
	assert.NoError(t, err)
	assert.Equal(t, testBackup.ID, retrieved.ID)
	assert.Equal(t, testBackup.ServerID, retrieved.ServerID)
	assert.Equal(t, testBackup.Status, retrieved.Status)

	// Add more tests for List, UpdateStatus, and Delete
}
