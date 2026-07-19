package rootfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectsTraversalAndSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "server")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	fsys, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	for _, name := range []string{"../server-evil/file", "/tmp/file", "..\\outside", "safe/../outside", "safe/.."} {
		if _, err := fsys.Open(name); err == nil {
			t.Fatalf("expected %q to fail", name)
		}
	}
	if _, err := Clean("server-evil/file"); err != nil {
		t.Fatalf("sibling-prefix path should remain a valid relative path: %v", err)
	}
}

func TestRejectsSymlinkParentEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	fsys, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := fsys.MkdirAll("escape/child", 0o750); err == nil {
		t.Fatal("expected symlink parent rejection")
	}
	if _, err := fsys.OpenFile("escape/file", os.O_CREATE|os.O_WRONLY, 0o640); err == nil {
		t.Fatal("expected symlink parent rejection")
	}
}

func TestRejectsReplacedParentSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o750); err != nil {
		t.Fatal(err)
	}
	fsys, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := os.Rename(parent, filepath.Join(root, "old-parent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Skip(err)
	}
	if err := fsys.WriteFile("parent/escaped.txt", []byte("no"), 0o640); err == nil {
		t.Fatal("expected replaced parent symlink to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created: %v", err)
	}
}

func TestAtomicWriteErrorsDoNotReplaceDestination(t *testing.T) {
	fsys, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := fsys.WriteFile("target.txt", []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := fsys.AtomicWrite("target.txt", strings.NewReader("oversize"), 3, 0o640); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if err := fsys.AtomicWriteExact("target.txt", strings.NewReader("short"), 10, 7, 0o640); !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected ErrSizeMismatch, got %v", err)
	}
	body, err := os.ReadFile(filepath.Join(fsys.Root(), "target.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "original" {
		t.Fatalf("destination changed after failed atomic write: %q", body)
	}
}
