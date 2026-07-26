// SPDX-License-Identifier: MIT
// Fallback filesystem operations for non-Linux platforms.

//go:build !linux

package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
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
// platforms, this falls back to a segment-by-segment openat(2) walk: each
// intermediate directory component is opened relative to the previously
// opened directory file descriptor with O_NOFOLLOW|O_DIRECTORY, and the final
// component is opened relative to the last directory fd with O_NOFOLLOW added
// to the caller-supplied flags. Because every step operates on an
// already-open fd rather than a path string that could be re-resolved, there
// is no window between a "check" (e.g. EvalSymlinks) and a later "open"
// during which an attacker who controls a symlink along the path could swap
// it out from under us (TOCTOU). New production server-root code should
// still prefer internal/rootfs, whose fallback rejects symlinks at every
// component and has had more extensive review.
//
// Residual risk: O_NOFOLLOW only guards against the final/intermediate
// components being (or becoming) symlinks; it does not prevent a
// non-symlink file from being replaced by another non-symlink file of the
// same name between opening its parent directory and opening it. That
// narrow race is accepted here in exchange for portability across non-Linux
// Unix targets, which lack openat2/RESOLVE_BENEATH.
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

	displayPath := filepath.Join(root, cleaned)

	// Open the root directory itself. The root is allowed to be a symlink
	// (e.g. macOS /var -> /private/var), matching prior behavior; only the
	// path *beneath* root is walked with O_NOFOLLOW below.
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: root, Err: err}
	}
	defer unix.Close(rootFD)

	if cleaned == "" {
		return openFinalComponentAt(rootFD, ".", displayPath, flag, mode)
	}

	segments := strings.Split(cleaned, string(filepath.Separator))

	// currentFD tracks the most recently opened directory fd. ownsCurrentFD
	// is false while currentFD == rootFD, since rootFD is already closed by
	// the defer above.
	currentFD := rootFD
	ownsCurrentFD := false
	defer func() {
		if ownsCurrentFD {
			unix.Close(currentFD)
		}
	}()

	for i, segment := range segments {
		if i == len(segments)-1 {
			return openFinalComponentAt(currentFD, segment, displayPath, flag, mode)
		}

		childFD, openErr := openatNoFollow(currentFD, segment, unix.O_RDONLY|unix.O_DIRECTORY)
		if openErr != nil {
			return nil, &os.PathError{Op: "open", Path: displayPath, Err: openErr}
		}
		if ownsCurrentFD {
			unix.Close(currentFD)
		}
		currentFD = childFD
		ownsCurrentFD = true
	}

	// Unreachable: segments is non-empty whenever cleaned != "".
	return nil, &os.PathError{Op: "open", Path: path, Err: errors.New("path escapes root directory")}
}

// openatNoFollow opens name relative to dirFD, rejecting the case where name
// is a symlink (the kernel returns ELOOP instead of following it).
func openatNoFollow(dirFD int, name string, flag int) (int, error) {
	for {
		fd, err := unix.Openat(dirFD, name, flag|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		return fd, nil
	}
}

// openFinalComponentAt opens the last path component relative to dirFD, with
// O_NOFOLLOW added to the caller-supplied flags. If the final component is
// unexpectedly a symlink at open time (for example, swapped in by an
// attacker after an earlier check elsewhere), the open fails with ELOOP
// instead of silently following it.
func openFinalComponentAt(dirFD int, name, displayPath string, flag int, mode uint32) (*os.File, error) {
	for {
		fd, err := unix.Openat(dirFD, name, flag|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: displayPath, Err: err}
		}
		return os.NewFile(uintptr(fd), displayPath), nil
	}
}
