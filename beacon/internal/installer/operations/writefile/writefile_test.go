package writefile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	op := &WriteFile{Dest: "test.txt", Content: "hello world"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteFileSubdir(t *testing.T) {
	dir := t.TempDir()
	op := &WriteFile{Dest: "sub/dir/file.txt", Content: "nested"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sub/dir/file.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "nested" {
		t.Fatalf("expected 'nested', got %q", string(data))
	}
}

func TestWriteFileFactory(t *testing.T) {
	op, err := factory([]byte(`{"dest": "out.txt", "content": "data"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wf, ok := op.(*WriteFile)
	if !ok {
		t.Fatal("expected *WriteFile type")
	}
	if wf.Dest != "out.txt" || wf.Content != "data" {
		t.Fatal("unexpected fields")
	}
}

func TestWriteFileFactoryMissingDest(t *testing.T) {
	_, err := factory([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing dest")
	}
}
