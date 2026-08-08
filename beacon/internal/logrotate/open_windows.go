//go:build windows

package logrotate

import (
	"errors"
	"os"
)

func openLogFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("refusing symbolic-link log path")
	}
	return os.OpenFile(path, flags, perm)
}
