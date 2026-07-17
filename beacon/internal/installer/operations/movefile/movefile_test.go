package movefile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &MoveFile{Source: "src.txt", Dest: "dst.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("expected source to be gone")
	}

	data, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}
}

func TestMoveFileFactory(t *testing.T) {
	op, err := factory([]byte(`{"source": "a", "dest": "b"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mf, ok := op.(*MoveFile)
	if !ok {
		t.Fatal("expected *MoveFile type")
	}
	if mf.Source != "a" || mf.Dest != "b" {
		t.Fatal("unexpected fields")
	}
}
