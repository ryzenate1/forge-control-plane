package removefile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveFile{Target: "target.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	op := &RemoveFile{Target: "missing.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveDirNonRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	op := &RemoveFile{Target: "subdir"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatal("expected dir to be removed")
	}
}

func TestRemoveDirRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(filepath.Join(sub, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveFile{Target: "subdir", Recursive: true}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatal("expected dir to be removed recursively")
	}
}

func TestRemoveFileFactory(t *testing.T) {
	op, err := factory([]byte(`{"target": "x.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rf, ok := op.(*RemoveFile)
	if !ok {
		t.Fatal("expected *RemoveFile type")
	}
	if rf.Target != "x.txt" {
		t.Fatalf("unexpected target: %s", rf.Target)
	}
}
