package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeManagerInitialState(t *testing.T) {
	mgr := NewUpgradeManager("1.0.0", "/tmp/beacon", t.TempDir())
	status := mgr.Status()
	if status.State != UpgradeStateIdle {
		t.Fatalf("expected idle, got %s", status.State)
	}
	if status.Version != "" {
		t.Fatalf("expected empty version, got %s", status.Version)
	}
}

func TestUpgradeManagerBeginInvalidVersion(t *testing.T) {
	mgr := NewUpgradeManager("1.0.0", "/tmp/beacon", t.TempDir())
	err := mgr.Begin(context.Background(), UpgradePayload{
		Version:     "",
		DownloadURL: "http://example.com/beacon",
		Checksum:    "",
	})
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestUpgradeManagerBackupAndRollback(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "beacon")
	if err := os.WriteFile(binPath, []byte("original binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr := NewUpgradeManager("1.0.0", binPath, dir)
	if err := mgr.backupCurrentBinary(); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(dir, "backups", "beacon", "beacon.1.0.0")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("backup was not created")
	}
	if err := os.WriteFile(binPath, []byte("upgraded binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr.currentVersion = "2.0.0"
	mgr.prevVersion = "1.0.0"
	mgr.state = UpgradeStateCompleted
	if err := mgr.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original binary" {
		t.Fatalf("expected original binary after rollback, got %s", data)
	}
}

func TestUpgradeManagerDoubleBegin(t *testing.T) {
	mgr := NewUpgradeManager("1.0.0", "/tmp/beacon", t.TempDir())
	mgr.state = UpgradeStateDownloading
	err := mgr.Begin(context.Background(), UpgradePayload{
		Version:     "2.0.0",
		DownloadURL: "http://example.com/beacon",
	})
	if err == nil {
		t.Fatal("expected ErrUpgradeInProgress for concurrent upgrade")
	}
}

func TestUpgradeManagerRollbackWithoutUpgrade(t *testing.T) {
	mgr := NewUpgradeManager("1.0.0", "/tmp/beacon", t.TempDir())
	err := mgr.Rollback(context.Background())
	if err == nil {
		t.Fatal("expected error for rollback without upgrade")
	}
}

func TestUpgradeManagerVersionIncompatibility(t *testing.T) {
	compat := CheckVersionCompatibility("", "")
	if compat.Compatible {
		t.Fatal("expected empty version to be incompatible")
	}
	compat2 := CheckVersionCompatibility("1.0.0", "")
	if !compat2.Compatible {
		t.Fatal("expected 1.0.0 to be compatible")
	}
	compat3 := CheckVersionCompatibility("0.0.1", "1.0.0")
	if compat3.Compatible {
		t.Fatal("expected 0.0.1 to be incompatible when API version is specified")
	}
}

func TestUpgradeManagerStatusAfterBegin(t *testing.T) {
	mgr := NewUpgradeManager("1.0.0", "/tmp/beacon", t.TempDir())
	mgr.version = "2.0.0"
	mgr.prevVersion = "1.0.0"
	mgr.state = UpgradeStateDownloading
	// startedAt is set during NewUpgradeManager
	status := mgr.Status()
	if status.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %s", status.Version)
	}
	if status.PrevVersion != "1.0.0" {
		t.Fatalf("expected prev version 1.0.0, got %s", status.PrevVersion)
	}
}
