package testutil

import (
	"context"
	"os"
	"testing"
)

func SkipIfNoDatabase(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
}

func Context() context.Context {
	return context.Background()
}
