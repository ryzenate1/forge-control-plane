package downloadfile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	op := &DownloadFile{URL: srv.URL, Dest: "downloaded.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "downloaded.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

func TestDownloadFileEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{})
	}))
	defer srv.Close()

	dir := t.TempDir()
	op := &DownloadFile{URL: srv.URL, Dest: "empty.txt"}
	if err := op.Execute(context.Background(), dir); err == nil {
		t.Fatal("expected error for empty download")
	}
}

func TestDownloadFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	op := &DownloadFile{URL: srv.URL, Dest: "missing.txt"}
	if err := op.Execute(context.Background(), dir); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDownloadFileFactory(t *testing.T) {
	raw := []byte(`{"url": "https://example.com/file", "dest": "out.jar"}`)
	op, err := factory(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dl, ok := op.(*DownloadFile)
	if !ok {
		t.Fatal("expected *DownloadFile type")
	}
	if dl.URL != "https://example.com/file" {
		t.Fatalf("unexpected url: %s", dl.URL)
	}
	if dl.Dest != "out.jar" {
		t.Fatalf("unexpected dest: %s", dl.Dest)
	}
}

func TestDownloadFileFactoryMissingArgs(t *testing.T) {
	_, err := factory([]byte(`{"url": ""}`))
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}
