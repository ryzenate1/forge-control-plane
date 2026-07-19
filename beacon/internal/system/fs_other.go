// SPDX-License-Identifier: MIT
// Fallback filesystem operations for non-Linux platforms.

//go:build !linux

package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrOpenat2NotSupported is returned when openat2 syscall operations are
// attempted on a platform that does not support them.
var ErrOpenat2NotSupported = errors.New("openat2 is not supported on this platform")

// IsOpenat2Supported always returns false on non-Linux platforms, as the
// openat2 syscall is Linux-specific (kernel 5.6+).
func IsOpenat2Supported() bool {
	return false
}

// OpenRoot is not supported on non-Linux platforms and always returns an error.
func OpenRoot(path string) (int, error) {
	return 0, &os.PathError{Op: "open", Path: path, Err: ErrOpenat2NotSupported}
}

// SafeOpen is not supported on non-Linux platforms and always returns an error.
func SafeOpen(rootFD int, name string, flag int, mode uint32) (int, error) {
	return 0, &os.PathError{Op: "openat2", Path: name, Err: ErrOpenat2NotSupported}
}

// SafeJoinOpen safely opens a file within a root directory. On non-Linux
// platforms, this falls back to userspace path validation. New production
// server-root code should use internal/rootfs, whose fallback rejects symlinks
// at every component. This legacy helper remains subject to pathname TOCTOU
// races and must not be used for attacker-controlled server paths.
func SafeJoinOpen(root, path string, flag int, mode uint32) (*os.File, error) {
	// Reject null bytes in the path.
	if strings.ContainsRune(path, 0) {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("invalid path: contains null byte")}
	}

	// Clean the requested path and reject absolute paths or parent traversals.
	cleaned := filepath.Clean(strings.TrimPrefix(path, "/"))
	if cleaned == "." {
		cleaned = ""
	}
	if filepath.IsAbs(path) || strings.HasPrefix(cleaned, "..") {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
	}

	// Resolve the root through symlinks so Rel comparison works correctly
	// on systems where directories are symlinked (e.g., macOS /var → /private/var).
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		// If root doesn't exist, use it as-is.
		resolvedRoot = root
	}

	target := filepath.Join(resolvedRoot, cleaned)

	// Verify the target is within the root by checking the relative path.
	rel, err := filepath.Rel(resolvedRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
	}

	// If the target exists, resolve it through symlinks and verify again.
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		resolvedRel, err := filepath.Rel(resolvedRoot, resolved)
		if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
			return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
		}
		target = resolved
	} else if !os.IsNotExist(err) {
		// For non-existence errors (e.g., permission), check the parent.
		parent := filepath.Dir(target)
		if parent != resolvedRoot {
			if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
				parentRel, err := filepath.Rel(resolvedRoot, resolvedParent)
				if err != nil || parentRel == ".." || strings.HasPrefix(parentRel, ".."+string(filepath.Separator)) {
					return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
				}
			}
		}
	}

	// Open the file with the validated path.
	f, err := os.OpenFile(target, flag, os.FileMode(mode))
	if err != nil {
		return nil, err
	}
	return f, nil
}
