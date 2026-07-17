package symlink

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &Symlink{Source: "actual.txt", Dest: "link.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	link := filepath.Join(dir, "link.txt")
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != src {
		t.Fatalf("expected symlink target %q, got %q", src, linkTarget)
	}
}

func TestSymlinkOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "actual.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLink := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(oldLink, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	op := &Symlink{Source: "actual.txt", Dest: "old.txt"}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	linkTarget, err := os.Readlink(oldLink)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != src {
		t.Fatalf("expected symlink target %q, got %q", src, linkTarget)
	}
}

func TestSymlinkFactory(t *testing.T) {
	op, err := factory([]byte(`{"source": "a", "dest": "b"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sl, ok := op.(*Symlink)
	if !ok {
		t.Fatal("expected *Symlink type")
	}
	if sl.Source != "a" || sl.Dest != "b" {
		t.Fatal("unexpected fields")
	}
}
