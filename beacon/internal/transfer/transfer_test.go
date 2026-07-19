package transfer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const transferTestServerID = "123e4567-e89b-12d3-a456-426614174000"

func TestStartUsesTransferOwnedContext(t *testing.T) {
	received := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "server.properties"), []byte("online-mode=true"), 0o600); err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	manager := NewManager()
	transfer, err := manager.Start(requestCtx, transferTestServerID, "source", "target", root, target.URL, "token", 0)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("transfer was cancelled with the request context")
	}
	waitForTransferStatus(t, transfer, StatusCompleted)
}

func TestStreamToTargetSeeksResumeOffset(t *testing.T) {
	var body string
	var contentLength int64
	var resumeHeader string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		body = string(payload)
		contentLength = r.ContentLength
		resumeHeader = r.Header.Get("X-Transfer-Resume-Offset")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	archivePath := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(archivePath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	transfer := &Transfer{ID: "transfer-id", ServerID: transferTestServerID, ResumeOffset: 4}
	if err := NewManager().streamToTarget(context.Background(), archivePath, target.URL, "token", transfer); err != nil {
		t.Fatal(err)
	}
	if body != "456789" || contentLength != 6 || resumeHeader != "4" {
		t.Fatalf("resume upload mismatch: body=%q length=%d header=%q", body, contentLength, resumeHeader)
	}
}

func TestStreamToTargetRejectsInvalidResumeOffset(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(archivePath, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int64{-1, 5} {
		transfer := &Transfer{ID: "transfer-id", ServerID: transferTestServerID, ResumeOffset: offset}
		err := NewManager().streamToTarget(context.Background(), archivePath, "http://127.0.0.1", "token", transfer)
		if err == nil || !strings.Contains(err.Error(), "resume offset") {
			t.Fatalf("expected offset %d to be rejected, got %v", offset, err)
		}
	}
}

func TestStartRejectsUnsupportedResume(t *testing.T) {
	_, err := NewManager().Start(context.Background(), transferTestServerID, "source", "target", t.TempDir(), "http://example.invalid", "token", 1)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected non-zero resume to be rejected, got %v", err)
	}
}

func waitForTransferStatus(t *testing.T, transfer *Transfer, want Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		transfer.mu.Lock()
		status := transfer.Status
		errText := transfer.Error
		transfer.mu.Unlock()
		if status == want {
			return
		}
		if status == StatusFailed {
			t.Fatalf("transfer failed: %s", errText)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("transfer did not reach status %q", want)
}
