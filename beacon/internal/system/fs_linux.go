// SPDX-License-Identifier: MIT
// Filesystem traversal protection using Linux's openat2 syscall.

//go:build linux

package system

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	// O_CLOEXEC ensures the file descriptor is closed on exec.
	// Go sets this in the os package, but we need to set it ourselves
	// when using unix syscalls directly.
	o_CLOEXEC = unix.O_CLOEXEC

	// O_LARGEFILE enables support for files larger than 2GB on 32-bit systems.
	// Go sets this for unix.Open and unix.Openat, but NOT for unix.Openat2.
	o_LARGEFILE = 0100000
)

var (
	openat2Supported     bool
	openat2SupportedOnce sync.Once
)

// IsOpenat2Supported tests if the openat2 syscall is available on the running
// kernel (requires Linux 5.6+). The result is cached after the first call.
func IsOpenat2Supported() bool {
	openat2SupportedOnce.Do(func() {
		fd, err := unix.Openat2(unix.AT_FDCWD, ".", &unix.OpenHow{
			Flags: unix.O_RDONLY | unix.O_CLOEXEC,
		})
		if err != nil {
			openat2Supported = false
			return
		}
		_ = unix.Close(fd)
		openat2Supported = true
	})
	return openat2Supported
}

// OpenRoot opens a directory and returns its file descriptor. The caller is
// responsible for closing the returned FD with unix.Close.
func OpenRoot(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return fd, nil
}

// SafeOpen wraps unix.Openat2 with RESOLVE_BENEATH to prevent filesystem
// traversal attacks. The RESOLVE_BENEATH flag instructs the kernel to disallow
// path resolution that would escape the directory referred to by rootFD,
// including via absolute symlinks or ".." components.
//
// This provides kernel-level protection against symlink-based TOCTOU attacks
// that userspace path validation cannot fully prevent.
func SafeOpen(rootFD int, name string, flag int, mode uint32) (int, error) {
	// Ensure O_CLOEXEC is set.
	if flag&o_CLOEXEC == 0 {
		flag |= o_CLOEXEC
	}
	// Ensure O_LARGEFILE is set. unix.Openat2 does not set this automatically
	// unlike unix.Open and unix.Openat.
	if flag&o_LARGEFILE == 0 {
		flag |= o_LARGEFILE
	}

	for {
		fd, err := unix.Openat2(rootFD, name, &unix.OpenHow{
			Flags: uint64(flag),
			Mode:  uint64(mode),
			// Keep resolution beneath rootFD, reject procfs-style magic links,
			// and reject symlinks at every path component. Callers that need
			// links must implement an explicit, separately reviewed policy.
			Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS,
		})
		switch {
		case err == nil:
			return fd, nil
		case err == unix.EINTR:
			// Retry on EINTR per https://go.dev/issue/11180 and
			// https://go.dev/issue/39237.
			continue
		case err == unix.EAGAIN:
			continue
		default:
			return 0, &os.PathError{Op: "openat2", Path: name, Err: err}
		}
	}
}

// SafeJoinOpen combines OpenRoot and SafeOpen to safely open a file beneath
// a root directory. It returns an *os.File that the caller is responsible for
// closing.
func SafeJoinOpen(root, path string, flag int, mode uint32) (*os.File, error) {
	rootFD, err := OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFD)

	fd, err := SafeOpen(rootFD, path, flag, mode)
	if err != nil {
		return nil, err
	}

	// Convert the raw file descriptor into an *os.File. os.NewFile does not
	// take ownership of the name, it is used only for diagnostics.
	return os.NewFile(uintptr(fd), root+"/"+path), nil
}
