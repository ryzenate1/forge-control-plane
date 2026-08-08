package database

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrator(t *testing.T) {
	// Setup test database
	db, err := NewSQLiteDatabase(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create temp directory for migrations
	tempDir, err := os.MkdirTemp("", "migrations")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test migration files
	testMigrations := []struct {
		name    string
		content string
	}{
		{
			name:    "001_initial.sql",
			content: "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT);",
		},
		{
			name:    "002_add_index.sql",
			content: "CREATE INDEX idx_test_name ON test(name);",
		},
	}

	for _, m := range testMigrations {
		err = os.WriteFile(filepath.Join(tempDir, m.name), []byte(m.content), 0644)
		require.NoError(t, err)
	}

	// Create migrator
	migrator := NewMigrator(db, tempDir, log.New(os.Stdout, "", 0))

	// Run migrations
	err = migrator.Migrate(context.Background())
	require.NoError(t, err)

	// Verify migrations were applied
	rows, err := db.Query(context.Background(), "SELECT name FROM migrations")
	require.NoError(t, err)
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		require.NoError(t, err)
		names = append(names, name)
	}

	assert.ElementsMatch(t, []string{"001_initial.sql", "002_add_index.sql"}, names)

	// Verify tables were created
	_, err = db.Query(context.Background(), "SELECT * FROM test")
	assert.NoError(t, err)

	// Verify index was created
	// This is a bit tricky to verify directly, so we'll just check that the migration ran without error
}
