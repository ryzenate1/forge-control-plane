package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupCronRemovesOldTransferArchives(t *testing.T) {
	tmpDir := os.TempDir()
	old := filepath.Join(tmpDir, "transfer-old.tar.gz")
	new := filepath.Join(tmpDir, "transfer-new.tar.gz")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(new, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(new)
	oldTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(old, oldTime, oldTime)

	cc := NewCleanupCron(t.TempDir())
	if err := cc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expected old transfer archive to be removed")
	}
	if _, err := os.Stat(new); err != nil {
		t.Fatal("expected new transfer archive to remain")
	}
}

func TestCleanupCronRemovesStaleUploads(t *testing.T) {
	dataDir := t.TempDir()
	uploadsDir := filepath.Join(dataDir, ".uploads")
	if err := os.MkdirAll(uploadsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(uploadsDir, "session-1")
	if err := os.WriteFile(stale, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(uploadsDir, "session-2")
	if err := os.WriteFile(fresh, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(stale, staleTime, staleTime)

	cc := NewCleanupCron(dataDir)
	if err := cc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("expected stale upload to be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("expected fresh upload to remain")
	}
}

func TestCleanupCronRemovesTempFiles(t *testing.T) {
	dataDir := t.TempDir()
	tmpSubdir := filepath.Join(dataDir, ".tmp")
	if err := os.MkdirAll(tmpSubdir, 0o750); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(tmpSubdir, "old-file")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(old, oldTime, oldTime)

	cc := NewCleanupCron(dataDir)
	if err := cc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expected old temp file to be removed")
	}
}

func TestCleanupCronEmptyDataDir(t *testing.T) {
	cc := NewCleanupCron("")
	if err := cc.Run(context.Background()); err != nil {
		t.Fatalf("expected no error for empty data dir, got: %v", err)
	}
}

func TestCleanupCronMissingDirs(t *testing.T) {
	cc := NewCleanupCron("/nonexistent/path")
	if err := cc.Run(context.Background()); err != nil {
		t.Fatalf("expected no error for missing dirs, got: %v", err)
	}
}

func TestCleanupCronRemovesOldStaleDirectories(t *testing.T) {
	dataDir := t.TempDir()
	uploadsDir := filepath.Join(dataDir, ".uploads")
	if err := os.MkdirAll(uploadsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	staleDir := filepath.Join(uploadsDir, "old-session")
	if err := os.MkdirAll(staleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "chunk-1"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-25 * time.Hour)
	os.Chtimes(staleDir, oldTime, oldTime)

	cc := NewCleanupCron(dataDir)
	if err := cc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatal("expected stale directory to be removed")
	}
}
