package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

func SkipIfNoDatabase(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("DATABASE_URL set but cannot open connection: %v; skipping integration test", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("DATABASE_URL set but database not reachable: %v; skipping integration test", err)
	}
}

func Context() context.Context {
	return context.Background()
}
