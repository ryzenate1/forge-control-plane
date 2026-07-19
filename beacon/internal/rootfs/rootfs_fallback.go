//go:build !linux

package rootfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var fallbackLocks sync.Map

// fallbackFS rejects symlinks at every observed component and serializes
// operations performed through a given root. Platforms without descriptor-
// relative APIs cannot prevent an external process from replacing a checked
// component before the subsequent pathname syscall; callers requiring the
// Linux openat2 guarantee must run on Linux 5.6 or newer.
type fallbackFS struct {
	root string
	mu   *sync.Mutex
}

func newPlatformFS(root string) (platformFS, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlink
	}
	if !info.IsDir() {
		return nil, errors.New("root is not a directory")
	}
	value, _ := fallbackLocks.LoadOrStore(root, &sync.Mutex{})
	return &fallbackFS{root: root, mu: value.(*sync.Mutex)}, nil
}

func (f *fallbackFS) close() error { return nil }

func (f *fallbackFS) checked(name string, allowMissingFinal bool) (string, error) {
	parts := strings.Split(name, "/")
	current := f.root
	for index, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && allowMissingFinal && index == len(parts)-1 {
				return current, nil
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrSymlink
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", errors.New("path component is not a directory")
		}
	}
	return current, nil
}

func (f *fallbackFS) open(name string, flags int, perm os.FileMode) (*os.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	allowMissing := flags&os.O_CREATE != 0
	target, err := f.checked(name, allowMissing)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(target, flags, perm)
}

func (f *fallbackFS) mkdirAll(name string, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	current := f.root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return ErrSymlink
			}
			if !info.IsDir() {
				return errors.New("path component is not a directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, perm); err != nil {
			return err
		}
	}
	return nil
}

func (f *fallbackFS) removeAll(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	target, err := f.checked(name, false)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func (f *fallbackFS) rename(oldName, newName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	oldPath, err := f.checked(oldName, false)
	if err != nil {
		return err
	}
	newPath, err := f.checked(newName, true)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (f *fallbackFS) chtimes(name string, atime, mtime time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	target, err := f.checked(name, false)
	if err != nil {
		return err
	}
	return os.Chtimes(target, atime, mtime)
}

func (f *fallbackFS) chmod(name string, mode os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	target, err := f.checked(name, false)
	if err != nil {
		return err
	}
	return os.Chmod(target, mode)
}
