package operations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ResolvePath(serverDir, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(serverDir, target))
}

func FileExists(serverDir, target string) (bool, error) {
	full := ResolvePath(serverDir, target)
	info, err := os.Stat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat %q: %w", full, err)
	}
	return info.Mode().IsRegular(), nil
}

func EnsureParentDir(path string) error {
	parent := filepath.Dir(path)
	if parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}

func PathExists(serverDir, target string) (bool, error) {
	full := ResolvePath(serverDir, target)
	_, err := os.Stat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat %q: %w", full, err)
	}
	return true, nil
}
