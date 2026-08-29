// SPDX-License-Identifier: MIT
// Filesystem operations for Windows platforms.

//go:build windows

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

// IsOpenat2Supported always returns false on Windows platforms.
func IsOpenat2Supported() bool {
	return false
}

// OpenRoot is not supported on Windows platforms and always returns an error.
func OpenRoot(path string) (int, error) {
	return 0, &os.PathError{Op: "open", Path: path, Err: ErrOpenat2NotSupported}
}

// SafeOpen is not supported on Windows platforms and always returns an error.
func SafeOpen(rootFD int, name string, flag int, mode uint32) (int, error) {
	return 0, &os.PathError{Op: "openat2", Path: name, Err: ErrOpenat2NotSupported}
}

// SafeJoinOpen safely opens a file within a root directory on Windows.
func SafeJoinOpen(root, path string, flag int, mode uint32) (*os.File, error) {
	if strings.ContainsRune(path, 0) {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("invalid path: contains null byte")}
	}

	cleanRel := filepath.Clean(strings.TrimPrefix(path, "/"))
	cleanRel = strings.TrimPrefix(cleanRel, "\\")
	if cleanRel == "." {
		cleanRel = ""
	}
	if filepath.IsAbs(path) || strings.HasPrefix(cleanRel, "..") {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
	}

	target := filepath.Join(root, cleanRel)
	rel, err := filepath.Rel(root, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
	}

	return os.OpenFile(target, flag, os.FileMode(mode))
}
