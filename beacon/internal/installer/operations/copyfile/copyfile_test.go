package copyfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &CopyFile{Source: "src.txt", Dest: "dst.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}
}

func TestCopyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "srcdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "srcdir", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "srcdir", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &CopyFile{Source: "srcdir", Dest: "dstdir"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "dstdir", "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(data) != "a" {
		t.Fatalf("expected 'a', got %q", string(data))
	}
}

func TestCopyFileFactory(t *testing.T) {
	op, err := factory([]byte(`{"source": "a", "dest": "b"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cf, ok := op.(*CopyFile)
	if !ok {
		t.Fatal("expected *CopyFile type")
	}
	if cf.Source != "a" || cf.Dest != "b" {
		t.Fatal("unexpected fields")
	}
}

func TestCopyFileFactoryMissingArgs(t *testing.T) {
	_, err := factory([]byte(`{"source": "a"}`))
	if err == nil {
		t.Fatal("expected error for missing dest")
	}
}
