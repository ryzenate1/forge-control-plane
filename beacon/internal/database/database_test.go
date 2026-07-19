package database

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteDatabase(t *testing.T) {
	// Setup test database
	db, err := NewSQLiteDatabase(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Test basic operations
	ctx := context.Background()

	// Create table
	_, err = db.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	require.NoError(t, err)

	// Insert data
	_, err = db.Exec(ctx, "INSERT INTO test (name) VALUES (?)", "test")
	require.NoError(t, err)

	// Query data
	rows, err := db.Query(ctx, "SELECT name FROM test")
	require.NoError(t, err)
	defer rows.Close()

	var name string
	for rows.Next() {
		err = rows.Scan(&name)
		require.NoError(t, err)
		assert.Equal(t, "test", name)
	}

	// Test transaction
	tx, err := db.BeginTx(ctx)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "INSERT INTO test (name) VALUES (?)", "transaction")
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	// Verify transaction was committed
	rows, err = db.Query(ctx, "SELECT COUNT(*) FROM test")
	require.NoError(t, err)
	defer rows.Close()

	var count int
	for rows.Next() {
		err = rows.Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	}
}
