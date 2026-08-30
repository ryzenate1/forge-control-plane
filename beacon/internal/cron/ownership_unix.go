//go:build unix

package cron

import (
	"os"
	"syscall"
)

// ownedByCurrentUser reports whether info describes a file owned by the
// current process's effective UID. Used by the cleanup cron's safety checks
// so it never deletes a file planted by another user in a shared temp
// directory, even if the name matches the expected pattern.
func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(stat.Uid) == os.Geteuid()
}
