//go:build windows

package backup

import (
	"os"
	"sync"
)

var winMu sync.Mutex

func flockLock(f *os.File) error {
	winMu.Lock()
	return nil
}

func flockUnlock(f *os.File) error {
	winMu.Unlock()
	return nil
}

func flockTryLock(f *os.File) error {
	if winMu.TryLock() {
		winMu.Unlock()
		return nil
	}
	return os.ErrInvalid
}
