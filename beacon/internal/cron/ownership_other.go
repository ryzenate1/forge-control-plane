//go:build !unix

package cron

import "os"

// ownedByCurrentUser has no reliable, dependency-free UID equivalent on
// non-Unix platforms via os.FileInfo, so it conservatively refuses the
// ownership check on those platforms; the caller then falls back to skipping
// the fallback-sweep deletion rather than risk removing a file it can't
// verify ownership of.
func ownedByCurrentUser(info os.FileInfo) bool {
	return false
}
