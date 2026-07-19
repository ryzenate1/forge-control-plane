package rootfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidPath  = errors.New("invalid rooted filesystem path")
	ErrSymlink      = errors.New("symbolic links are not permitted")
	ErrRoot         = errors.New("operation on filesystem root is not permitted")
	ErrTooLarge     = errors.New("file exceeds size limit")
	ErrSizeMismatch = errors.New("file size does not match expected length")
)

// FS confines all operations to Root. Paths passed to methods are slash-
// separated and relative to Root; symbolic links are never followed.
type FS struct {
	root string
	impl platformFS
}

type platformFS interface {
	close() error
	open(name string, flags int, perm os.FileMode) (*os.File, error)
	mkdirAll(name string, perm os.FileMode) error
	removeAll(name string) error
	rename(oldName, newName string) error
	chmod(name string, mode os.FileMode) error
	chtimes(name string, atime, mtime time.Time) error
}

func New(root string) (*FS, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	impl, err := newPlatformFS(absolute)
	if err != nil {
		return nil, err
	}
	return &FS{root: absolute, impl: impl}, nil
}

func (f *FS) Close() error { return f.impl.close() }
func (f *FS) Root() string { return f.root }

func Clean(name string) (string, error) {
	if strings.ContainsRune(name, 0) || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasDrivePrefix(name) {
		return "", ErrInvalidPath
	}
	for _, component := range strings.Split(name, "/") {
		if component == ".." {
			return "", ErrInvalidPath
		}
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

func hasDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	return (name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')
}

func (f *FS) Open(name string) (*os.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

func (f *FS) OpenFile(name string, flags int, perm os.FileMode) (*os.File, error) {
	clean, err := Clean(name)
	if err != nil {
		return nil, err
	}
	return f.impl.open(clean, flags, perm)
}

func (f *FS) Stat(name string) (fs.FileInfo, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.ReadDir(-1)
}

func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	clean, err := Clean(name)
	if err != nil {
		return err
	}
	if clean == "" {
		return nil
	}
	return f.impl.mkdirAll(clean, perm)
}

func (f *FS) RemoveAll(name string) error {
	clean, err := Clean(name)
	if err != nil {
		return err
	}
	if clean == "" {
		return ErrRoot
	}
	return f.impl.removeAll(clean)
}

func (f *FS) Rename(oldName, newName string) error {
	oldClean, err := Clean(oldName)
	if err != nil || oldClean == "" {
		return ErrInvalidPath
	}
	newClean, err := Clean(newName)
	if err != nil || newClean == "" {
		return ErrInvalidPath
	}
	return f.impl.rename(oldClean, newClean)
}

func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	clean, err := Clean(name)
	if err != nil || clean == "" {
		return ErrInvalidPath
	}
	return f.impl.chtimes(clean, atime, mtime)
}

func (f *FS) Chmod(name string, mode os.FileMode) error {
	clean, err := Clean(name)
	if err != nil || clean == "" {
		return ErrInvalidPath
	}
	return f.impl.chmod(clean, mode&os.ModePerm)
}

func (f *FS) WriteFile(name string, body []byte, perm os.FileMode) error {
	return f.AtomicWrite(name, bytes.NewReader(body), int64(len(body)), perm)
}

// AtomicWrite writes into a sibling temporary file and atomically renames it.
// max is an upper bound when non-negative; an error is returned if the stream
// exceeds it.
func (f *FS) AtomicWrite(name string, r io.Reader, max int64, perm os.FileMode) error {
	return f.atomicWrite(name, r, max, -1, perm)
}

// AtomicWriteExact is AtomicWrite with an additional exact-length check. The
// destination is not replaced unless the stream ends at exactly expected bytes.
func (f *FS) AtomicWriteExact(name string, r io.Reader, max, expected int64, perm os.FileMode) error {
	if expected < 0 || (max >= 0 && expected > max) {
		return ErrSizeMismatch
	}
	return f.atomicWrite(name, r, max, expected, perm)
}

func (f *FS) atomicWrite(name string, r io.Reader, max, expected int64, perm os.FileMode) error {
	writer, err := f.CreateAtomic(name, perm)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = writer.Abort()
		}
	}()
	reader := r
	if max >= 0 {
		reader = io.LimitReader(r, max+1)
	}
	written, err := io.Copy(writer, reader)
	if err != nil {
		return err
	}
	if max >= 0 && written > max {
		return ErrTooLarge
	}
	if expected >= 0 && written != expected {
		return ErrSizeMismatch
	}
	if err := writer.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

// AtomicFile supports random-access writes to a temporary sibling and commits
// them with a descriptor-relative rename when closed.
type AtomicFile struct {
	fs     *FS
	file   *os.File
	temp   string
	target string
	closed bool
}

func (f *FS) CreateAtomic(name string, perm os.FileMode) (*AtomicFile, error) {
	clean, err := Clean(name)
	if err != nil || clean == "" {
		return nil, ErrInvalidPath
	}
	parent := path.Dir(clean)
	if parent == "." {
		parent = ""
	}
	if err := f.MkdirAll(parent, 0o750); err != nil {
		return nil, err
	}
	temp := path.Join(parent, ".rootfs-tmp-"+randomName())
	file, err := f.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR, perm)
	if err != nil {
		return nil, err
	}
	return &AtomicFile{fs: f, file: file, temp: temp, target: clean}, nil
}

func (f *AtomicFile) Write(p []byte) (int, error)               { return f.file.Write(p) }
func (f *AtomicFile) WriteAt(p []byte, off int64) (int, error)  { return f.file.WriteAt(p, off) }
func (f *AtomicFile) Seek(off int64, whence int) (int64, error) { return f.file.Seek(off, whence) }
func (f *AtomicFile) Truncate(size int64) error                 { return f.file.Truncate(size) }

func (f *AtomicFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	if err := f.file.Sync(); err != nil {
		_ = f.file.Close()
		_ = f.fs.RemoveAll(f.temp)
		return err
	}
	if err := f.file.Close(); err != nil {
		_ = f.fs.RemoveAll(f.temp)
		return err
	}
	if err := f.fs.Rename(f.temp, f.target); err != nil {
		_ = f.fs.RemoveAll(f.temp)
		return err
	}
	return nil
}

func (f *AtomicFile) Abort() error {
	if !f.closed {
		f.closed = true
		_ = f.file.Close()
	}
	return f.fs.RemoveAll(f.temp)
}

// Usage returns the total size of non-symlink, non-directory entries beneath
// the opened root without resolving paths through the root pathname.
func (f *FS) Usage() (int64, error) {
	return f.usageDir("")
}

func (f *FS) usageDir(directory string) (int64, error) {
	entries, err := f.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := path.Join(directory, entry.Name())
		info, err := f.Stat(name)
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			size, err := f.usageDir(name)
			if err != nil {
				return 0, err
			}
			if size > int64(^uint64(0)>>1)-total {
				return 0, errors.New("filesystem usage overflow")
			}
			total += size
			continue
		}
		if info.Size() < 0 || info.Size() > int64(^uint64(0)>>1)-total {
			return 0, errors.New("filesystem usage overflow")
		}
		total += info.Size()
	}
	return total, nil
}

func (f *FS) Copy(source, destination string, perm os.FileMode, max int64) (int64, error) {
	src, err := f.Open(source)
	if err != nil {
		return 0, err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("source is not a regular file")
	}
	if max >= 0 && info.Size() > max {
		return 0, ErrTooLarge
	}
	if perm == 0 {
		perm = info.Mode().Perm()
	}
	if err := f.AtomicWriteExact(destination, src, info.Size(), info.Size(), perm); err != nil {
		return 0, err
	}
	return info.Size(), nil
}
