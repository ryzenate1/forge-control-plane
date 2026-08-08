package operations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(serverDir, target string) (string, error) {
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("absolute path %q is not allowed", target)
	}
	cleanedDir, err := filepath.Abs(serverDir)
	if err != nil {
		return "", fmt.Errorf("resolve server directory: %w", err)
	}
	cleanedDir, err = filepath.EvalSymlinks(cleanedDir)
	if err != nil {
		return "", fmt.Errorf("resolve server directory symlinks: %w", err)
	}
	resolved := filepath.Clean(filepath.Join(cleanedDir, target))
	if !pathWithin(cleanedDir, resolved) {
		return "", fmt.Errorf("path %q escapes server directory", target)
	}

	// Reject every existing symlink component. Merely cleaning the string is
	// insufficient: "plugins/link/config" can otherwise escape through a link
	// after the containment check.
	rel, err := filepath.Rel(cleanedDir, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}
	current := cleanedDir
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("inspect path component %q: %w", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q contains symlink component %q", target, component)
		}
	}
	return resolved, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func FileExists(serverDir, target string) (bool, error) {
	full, err := ResolvePath(serverDir, target)
	if err != nil {
		return false, err
	}
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
	return os.MkdirAll(parent, 0o750)
}

func SyncDirectory(path string) error {
	handle, err := os.Open(path)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func PathExists(serverDir, target string) (bool, error) {
	full, err := ResolvePath(serverDir, target)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat %q: %w", full, err)
	}
	return true, nil
}
