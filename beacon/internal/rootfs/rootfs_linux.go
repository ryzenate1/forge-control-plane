//go:build linux

package rootfs

import (
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type linuxFS struct {
	rootfd int
}

func newPlatformFS(root string) (platformFS, error) {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return &linuxFS{rootfd: fd}, nil
}

func (f *linuxFS) close() error { return unix.Close(f.rootfd) }

func (f *linuxFS) openRelative(dirfd int, name string, flags int, perm os.FileMode) (int, error) {
	if name == "" {
		return unix.Dup(dirfd)
	}
	how := &unix.OpenHow{
		Flags:   uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Mode:    uint64(perm.Perm()),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS,
	}
	for {
		fd, err := unix.Openat2(dirfd, name, how)
		switch err {
		case nil:
			return fd, nil
		case unix.EINTR, unix.EAGAIN:
			continue
		case unix.ENOSYS:
			return f.openatWalk(dirfd, name, flags, perm)
		default:
			return -1, err
		}
	}
}

func (f *linuxFS) openatWalk(dirfd int, name string, flags int, perm os.FileMode) (int, error) {
	parts := strings.Split(name, "/")
	current, err := unix.Dup(dirfd)
	if err != nil {
		return -1, err
	}
	for _, component := range parts[:len(parts)-1] {
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		current = next
	}
	fd, err := unix.Openat(current, parts[len(parts)-1], flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	_ = unix.Close(current)
	return fd, err
}

func (f *linuxFS) open(name string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := f.openRelative(f.rootfd, name, flags, perm)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (f *linuxFS) openDir(name string) (int, error) {
	return f.openRelative(f.rootfd, name, unix.O_RDONLY|unix.O_DIRECTORY, 0)
}

func (f *linuxFS) parent(name string) (int, string, error) {
	dir, base := path.Split(name)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		fd, err := unix.Dup(f.rootfd)
		return fd, base, err
	}
	fd, err := f.openDir(dir)
	return fd, base, err
}

func (f *linuxFS) mkdirAll(name string, perm os.FileMode) error {
	current, err := unix.Dup(f.rootfd)
	if err != nil {
		return err
	}
	for _, component := range strings.Split(name, "/") {
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr == unix.ENOENT {
			if err := unix.Mkdirat(current, component, uint32(perm.Perm())); err != nil && err != unix.EEXIST {
				_ = unix.Close(current)
				return err
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return openErr
		}
		_ = unix.Close(current)
		current = next
	}
	return unix.Close(current)
}

func (f *linuxFS) removeAll(name string) error {
	parent, base, err := f.parent(name)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	defer unix.Close(parent)
	return removeAt(parent, base)
}

func removeAt(parent int, name string) error {
	if err := unix.Unlinkat(parent, name, 0); err == nil || err == unix.ENOENT {
		return nil
	} else if err != unix.EISDIR && err != unix.EPERM && err != unix.EACCES {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if err == unix.ENOENT {
			return nil
		}
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return unix.Unlinkat(parent, name, 0)
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	for {
		names, readErr := file.Readdirnames(256)
		for _, child := range names {
			if err := removeAt(fd, child); err != nil {
				_ = file.Close()
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
}

func (f *linuxFS) rename(oldName, newName string) error {
	oldParent, oldBase, err := f.parent(oldName)
	if err != nil {
		return err
	}
	defer unix.Close(oldParent)
	newParent, newBase, err := f.parent(newName)
	if err != nil {
		return err
	}
	defer unix.Close(newParent)
	return unix.Renameat(oldParent, oldBase, newParent, newBase)
}

func (f *linuxFS) chtimes(name string, atime, mtime time.Time) error {
	file, err := f.open(name, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return unix.Futimes(int(file.Fd()), []unix.Timeval{unix.NsecToTimeval(atime.UnixNano()), unix.NsecToTimeval(mtime.UnixNano())})
}

func (f *linuxFS) chmod(name string, mode os.FileMode) error {
	file, err := f.open(name, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Chmod(mode.Perm())
}
