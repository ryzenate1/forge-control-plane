package backup

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalBackupLifecycleAndRestore(t *testing.T) {
	base := t.TempDir()
	serverRoot := filepath.Join(base, "servers", "server-one")
	backupRoot := filepath.Join(base, "backups")
	if err := os.MkdirAll(filepath.Join(serverRoot, "world"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "world", "level.dat"), []byte("original world"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "ignored.log"), []byte("ignore me"), 0o640); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLocalBackup(backupRoot)
	if err != nil {
		t.Fatal(err)
	}

	created, err := adapter.Create(context.Background(), serverRoot, "server-one", "backup-one.zip", []string{"*.log"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Checksum == "" || created.Size <= 0 || created.Adapter != LocalAdapter {
		t.Fatalf("incomplete backup metadata: %+v", created)
	}
	if _, err := os.Stat(filepath.Join(serverRoot, ".backups")); !os.IsNotExist(err) {
		t.Fatalf("backup storage was exposed in server root: %v", err)
	}
	archivePath := filepath.Join(backupRoot, "server-one", "backup-one.zip")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive not in daemon backup root: %v", err)
	}

	listed, err := adapter.List("server-one")
	if err != nil || len(listed) != 1 || listed[0].Checksum != created.Checksum {
		t.Fatalf("unexpected list result: %+v, %v", listed, err)
	}
	download, err := adapter.Download("server-one", "backup-one.zip")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(download)
	closeErr := download.Close()
	if err != nil || closeErr != nil || int64(len(body)) != created.Size {
		t.Fatalf("download mismatch: bytes=%d read=%v close=%v", len(body), err, closeErr)
	}

	if err := os.WriteFile(filepath.Join(serverRoot, "world", "level.dat"), []byte("changed"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "new.txt"), []byte("remove on restore"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Restore(context.Background(), "server-one", "backup-one.zip", serverRoot, true); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(serverRoot, "world", "level.dat"))
	if err != nil || string(restored) != "original world" {
		t.Fatalf("restore content mismatch: %q, %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(serverRoot, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale live data survived restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(serverRoot, "ignored.log")); !os.IsNotExist(err) {
		t.Fatalf("ignored file was restored: %v", err)
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("restore removed backup storage: %v", err)
	}

	if err := adapter.Delete("server-one", "backup-one.zip"); err != nil {
		t.Fatal(err)
	}
	listed, err = adapter.List("server-one")
	if err != nil || len(listed) != 0 {
		t.Fatalf("backup was not deleted: %+v, %v", listed, err)
	}
}

func TestLocalRestoreWithoutTruncation(t *testing.T) {
	backupRoot := t.TempDir()
	serverRoot := t.TempDir()
	adapter, err := NewLocalBackup(backupRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Create original file
	if err := os.WriteFile(filepath.Join(serverRoot, "original.txt"), []byte("original content"), 0o640); err != nil {
		t.Fatal(err)
	}

	// Create backup
	created, err := adapter.Create(context.Background(), serverRoot, "server-one", "backup-one.zip", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Modify original and add a new file that should survive non-truncate restore
	if err := os.WriteFile(filepath.Join(serverRoot, "original.txt"), []byte("changed content"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "survivor.txt"), []byte("i will survive"), 0o640); err != nil {
		t.Fatal(err)
	}

	// Restore with truncate = false
	if err := adapter.Restore(context.Background(), "server-one", created.Name, serverRoot, false); err != nil {
		t.Fatal(err)
	}

	// Verify original file is restored to backup state
	body, err := os.ReadFile(filepath.Join(serverRoot, "original.txt"))
	if err != nil || string(body) != "original content" {
		t.Fatalf("original file not restored correctly: %q, %v", body, err)
	}

	// Verify survivor file still exists (not truncated)
	body, err = os.ReadFile(filepath.Join(serverRoot, "survivor.txt"))
	if err != nil || string(body) != "i will survive" {
		t.Fatalf("survivor file was lost or corrupted: %q, %v", body, err)
	}
}

func TestLocalRestoreRejectsMaliciousArchivesAndPreservesLiveData(t *testing.T) {
	tests := map[string][]testZipEntry{
		"traversal":         {{name: "../escape", body: "bad", mode: 0o640}},
		"absolute":          {{name: "/escape", body: "bad", mode: 0o640}},
		"prefix collision":  {{name: "data", body: "file", mode: 0o640}, {name: "data/child", body: "bad", mode: 0o640}},
		"reverse collision": {{name: "data/child", body: "bad", mode: 0o640}, {name: "data", body: "file", mode: 0o640}},
		"symlink":           {{name: "link", body: "target", mode: os.ModeSymlink | 0o777}},
		"device":            {{name: "device", body: "bad", mode: os.ModeDevice | 0o600}},
		"duplicate":         {{name: "same", body: "one", mode: 0o640}, {name: "same", body: "two", mode: 0o640}},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			serverRoot := filepath.Join(base, "servers", "server-one")
			backupRoot := filepath.Join(base, "backups")
			if err := os.MkdirAll(serverRoot, 0o750); err != nil {
				t.Fatal(err)
			}
			livePath := filepath.Join(serverRoot, "live.txt")
			if err := os.WriteFile(livePath, []byte("keep me"), 0o640); err != nil {
				t.Fatal(err)
			}
			adapter, err := NewLocalBackup(backupRoot)
			if err != nil {
				t.Fatal(err)
			}
			archivePath := filepath.Join(backupRoot, "server-one", "malicious.zip")
			if err := writeTestZip(archivePath, entries); err != nil {
				t.Fatal(err)
			}
			if err := adapter.Restore(context.Background(), "server-one", "malicious.zip", serverRoot, true); err == nil {
				t.Fatal("malicious archive restore unexpectedly succeeded")
			}
			body, err := os.ReadFile(livePath)
			if err != nil || string(body) != "keep me" {
				t.Fatalf("failed restore changed live data: %q, %v", body, err)
			}
			if _, err := os.Stat(archivePath); err != nil {
				t.Fatalf("failed restore removed backup: %v", err)
			}
		})
	}
}

func TestLocalRestoreChecksumMismatchPreservesLiveData(t *testing.T) {
	base := t.TempDir()
	serverRoot := filepath.Join(base, "servers", "server-one")
	backupRoot := filepath.Join(base, "backups")
	if err := os.MkdirAll(serverRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(serverRoot, "live.txt")
	if err := os.WriteFile(livePath, []byte("keep me"), 0o640); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLocalBackup(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Create(context.Background(), serverRoot, "server-one", "backup.zip", nil); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(backupRoot, "server-one", "backup.zip")
	file, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("tamper")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("tamper archive: %v, %v", writeErr, closeErr)
	}
	err = adapter.Restore(context.Background(), "server-one", "backup.zip", serverRoot, true)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	body, readErr := os.ReadFile(livePath)
	if readErr != nil || string(body) != "keep me" {
		t.Fatalf("checksum failure changed live data: %q, %v", body, readErr)
	}
}

func TestLocalBackupMigratesLegacyStorageSafely(t *testing.T) {
	base := t.TempDir()
	dataRoot := filepath.Join(base, "servers")
	legacyArchive := filepath.Join(dataRoot, "server-one", ".backups", "legacy.zip")
	if err := writeTestZip(legacyArchive, []testZipEntry{{name: "world.dat", body: "world", mode: 0o640}}); err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(base, "backups")
	adapter, err := NewLocalBackup(backupRoot, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	backups, err := adapter.List("server-one")
	if err != nil || len(backups) != 1 || backups[0].Name != "legacy.zip" || backups[0].Checksum == "" {
		t.Fatalf("legacy backup was not migrated: %+v, %v", backups, err)
	}
	if _, err := os.Stat(filepath.Join(backupRoot, "server-one", "legacy.zip")); err != nil {
		t.Fatalf("migrated archive missing from daemon root: %v", err)
	}
	if _, err := os.Stat(legacyArchive); !os.IsNotExist(err) {
		t.Fatalf("legacy user-accessible archive was not removed: %v", err)
	}
}

func TestRecoverRestoreJournalsRestoresOriginalLiveData(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "servers")
	if err := os.MkdirAll(dataRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	serverRoot := filepath.Join(dataRoot, "server-one")
	rollback := filepath.Join(dataRoot, ".server-one.rollback-test")
	staging := filepath.Join(dataRoot, ".server-one.restore-test")
	if err := os.MkdirAll(rollback, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rollback, "live.txt"), []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o750); err != nil {
		t.Fatal(err)
	}
	journalPath := restoreJournalPath(serverRoot)
	if err := writeRestoreJournal(journalPath, restoreJournal{Staging: staging, Rollback: rollback, Phase: "live-moved"}); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRestoreJournals(dataRoot); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(serverRoot, "live.txt"))
	if err != nil || string(body) != "original" {
		t.Fatalf("original data was not recovered: %q, %v", body, err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("restore staging was not cleaned: %v", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("restore journal was not cleaned: %v", err)
	}
}

func TestLocalCreateRejectsBackupRootInsideServerRoot(t *testing.T) {
	serverRoot := t.TempDir()
	adapter, err := NewLocalBackup(filepath.Join(serverRoot, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Create(context.Background(), serverRoot, "server-one", "backup.zip", nil)
	if err == nil || !strings.Contains(err.Error(), "must be separate") {
		t.Fatalf("expected storage separation error, got %v", err)
	}
}

type testZipEntry struct {
	name string
	body string
	mode os.FileMode
}

func writeTestZip(archivePath string, entries []testZipEntry) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o750); err != nil {
		return err
	}
	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetMode(entry.mode)
		part, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
		if _, err := io.WriteString(part, entry.body); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
