//go:build linux

package rootfs

import (
	"os"
	"path/filepath"
	"testing"

	"gamepanel/beacon/internal/system"
)

func TestOpenat2RejectsSymlinkAndKeepsRootDescriptor(t *testing.T) {
	if !system.IsOpenat2Supported() {
		t.Skip("openat2 unavailable on this kernel")
	}
	base := t.TempDir()
	root := filepath.Join(base, "root")
	moved := filepath.Join(base, "moved")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	fsys, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()

	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile("inside.txt", []byte("safe"), 0o640); err != nil {
		t.Fatalf("descriptor-relative write failed after root pathname replacement: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, "inside.txt")); err != nil {
		t.Fatalf("write did not remain attached to opened root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "inside.txt")); !os.IsNotExist(err) {
		t.Fatalf("write escaped through replacement root: %v", err)
	}
}
