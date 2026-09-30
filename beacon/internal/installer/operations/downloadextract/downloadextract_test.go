package downloadextract

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func assertNoSpoolFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gamepanel-dl-") {
			t.Fatalf("failed download left spool file %q", entry.Name())
		}
	}
}

func TestDownloadToTempEnforcesLimitAndCleansUp(t *testing.T) {
	op := &DownloadExtract{MaxBytes: 1024}
	dir := t.TempDir()
	_, err := op.downloadToTemp(io.LimitReader(neverEndingByte('x'), 1024+256), dir)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size-limit error, got %v", err)
	}
	assertNoSpoolFiles(t, dir)
}

func TestDownloadToTempCleansUpOnReadError(t *testing.T) {
	op := &DownloadExtract{MaxBytes: 1024}
	dir := t.TempDir()
	failing := &failingReader{err: errors.New("connection reset")}
	_, err := op.downloadToTemp(failing, dir)
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("expected read error, got %v", err)
	}
	assertNoSpoolFiles(t, dir)
}

func TestDownloadToTempWritesCompletePayload(t *testing.T) {
	op := &DownloadExtract{MaxBytes: 4096}
	dir := t.TempDir()
	path, err := op.downloadToTemp(strings.NewReader("payload"), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Fatalf("unexpected spooled payload %q", body)
	}
}

type neverEndingByte byte

func (b neverEndingByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

type failingReader struct{ err error }

func (r *failingReader) Read([]byte) (int, error) { return 0, r.err }
