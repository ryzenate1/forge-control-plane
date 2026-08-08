package transfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func protocolClaims(direction string) CredentialClaims {
	return CredentialClaims{
		Version: ProtocolVersion, MigrationID: "migration-1", ServerID: "server-1",
		SourceNodeID: "source-1", TargetNodeID: "target-1", Direction: direction,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
}

func registerProtocolCredential(t *testing.T, engine *Engine, direction, credential string) {
	t.Helper()
	if err := engine.Register(CredentialRegistration{Claims: protocolClaims(direction), CredentialHash: HashCredential(credential)}); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolCredentialScopeExpiryRotationAndReplay(t *testing.T) {
	engine, err := NewProtocolEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "first")
	if _, err := engine.Authorize("migration-1", DirectionSourceControl, "first"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-scope credential accepted: %v", err)
	}
	if _, err := engine.Authorize("migration-1", DirectionDestinationUpload, "wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong credential accepted: %v", err)
	}
	claims := protocolClaims(DirectionDestinationUpload)
	claims.ExpiresAt = time.Now().Add(-time.Second)
	if err := engine.Register(CredentialRegistration{Claims: claims, CredentialHash: HashCredential("expired")}); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired registration accepted: %v", err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "rotated")
	if _, err := engine.Authorize("migration-1", DirectionDestinationUpload, "first"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("rotated credential remained valid: %v", err)
	}
	archive := makeTarGz(t, map[string]string{"server.properties": "ok"})
	checksum := shaHex(archive)
	if _, err := engine.AppendDestination(context.Background(), "migration-1", "rotated", 0, int64(len(archive)), checksum, bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RestoreDestination(context.Background(), "migration-1", "rotated"); err != nil {
		t.Fatal(err)
	}
	if err := engine.FinalizeDestination("migration-1", "rotated"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Authorize("migration-1", DirectionDestinationUpload, "rotated"); !errors.Is(err, ErrReplayed) {
		t.Fatalf("consumed credential replay accepted: %v", err)
	}
}

func TestProtocolInterruptedUploadActuallyAppendsAtNegotiatedOffset(t *testing.T) {
	engine, err := NewProtocolEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "upload")
	payload := []byte("0123456789")
	checksum := shaHex(payload)
	partial, err := engine.AppendDestination(context.Background(), "migration-1", "upload", 0, int64(len(payload)), checksum, bytes.NewReader(payload[:4]))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Offset != 4 || partial.Phase != "uploading" {
		t.Fatalf("unexpected partial metadata: %+v", partial)
	}
	negotiated, err := engine.DestinationOffset("migration-1", "upload")
	if err != nil {
		t.Fatal(err)
	}
	if negotiated.Offset != 4 {
		t.Fatalf("offset=%d, want 4", negotiated.Offset)
	}
	complete, err := engine.AppendDestination(context.Background(), "migration-1", "upload", negotiated.Offset, int64(len(payload)), checksum, bytes.NewReader(payload[4:]))
	if err != nil {
		t.Fatal(err)
	}
	if complete.Offset != int64(len(payload)) || complete.Phase != "verified" {
		t.Fatalf("unexpected complete metadata: %+v", complete)
	}
	body, err := os.ReadFile(engine.incomingPath("migration-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("destination did not append resumed bytes: %q", body)
	}
}

func TestProtocolRejectsWrongOffsetAndChecksum(t *testing.T) {
	engine, err := NewProtocolEngine(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "upload")
	payload := []byte("payload")
	checksum := shaHex(payload)
	if _, err := engine.AppendDestination(context.Background(), "migration-1", "upload", 1, int64(len(payload)), checksum, bytes.NewReader(payload)); !errors.Is(err, ErrOffsetMismatch) {
		t.Fatalf("wrong offset accepted: %v", err)
	}
	if _, err := engine.AppendDestination(context.Background(), "migration-1", "upload", 0, int64(len(payload)), shaHex([]byte("other")), bytes.NewReader(payload)); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("checksum mismatch accepted: %v", err)
	}
}

func TestProtocolExtractionFailurePreservesLiveDestination(t *testing.T) {
	dataDir := t.TempDir()
	canonical := filepath.Join(dataDir, "server-1")
	if err := os.MkdirAll(canonical, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "live.txt"), []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "upload")
	malformed := []byte("not-a-gzip-archive")
	if _, err := engine.AppendDestination(context.Background(), "migration-1", "upload", 0, int64(len(malformed)), shaHex(malformed), bytes.NewReader(malformed)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RestoreDestination(context.Background(), "migration-1", "upload"); err == nil {
		t.Fatal("expected extraction failure")
	}
	body, err := os.ReadFile(filepath.Join(canonical, "live.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "live" {
		t.Fatalf("live destination changed: %q", body)
	}
}

func TestProtocolCancellationRollsBackActivatedDestinationAndCleansTerminalMemory(t *testing.T) {
	dataDir := t.TempDir()
	canonical := filepath.Join(dataDir, "server-1")
	if err := os.MkdirAll(canonical, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionDestinationUpload, "upload")
	archive := makeTarGz(t, map[string]string{"new.txt": "new"})
	if _, err := engine.AppendDestination(context.Background(), "migration-1", "upload", 0, int64(len(archive)), shaHex(archive), bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.RestoreDestination(context.Background(), "migration-1", "upload"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(canonical, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := engine.Cancel("migration-1"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(canonical, "old.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "old" {
		t.Fatalf("rollback content=%q", body)
	}
	if engine.ActiveCount() != 0 {
		t.Fatalf("terminal active entries=%d", engine.ActiveCount())
	}
	if _, err := os.Stat(engine.transferDir("migration-1")); !os.IsNotExist(err) {
		t.Fatalf("terminal staging retained: %v", err)
	}
}

func TestProtocolSourceArchiveUsesPteroignore(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "server-1")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pteroignore"), []byte("secret.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine, DirectionSourceControl, "source")
	meta, err := engine.PrepareSource(context.Background(), "migration-1", "source")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "archived" || meta.ArchiveSize == 0 || meta.Checksum == "" {
		t.Fatalf("bad archive metadata: %+v", meta)
	}
	entries := readTarGz(t, engine.archivePath("migration-1"))
	if _, exists := entries["secret.txt"]; exists {
		t.Fatal("ignored file was archived")
	}
	if string(entries["keep.txt"]) != "keep" {
		t.Fatalf("kept file missing: %v", entries)
	}
}

func shaHex(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func readTarGz(t *testing.T, path string) map[string][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string][]byte{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, header.Size)
		if _, err := io.ReadFull(tr, body); err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = body
	}
	return entries
}
