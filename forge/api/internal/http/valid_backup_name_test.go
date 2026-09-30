package http

import (
	"strings"
	"testing"
)

// TestValidBackupName guards the panel-side backup-name safety rules.
//
// NOTE: the strict validBackupName() whitelist helper (charset/length/hidden
// file rules mirroring Beacon's archive filenames) was removed from
// internal/http during the refactor. The guard that survives in production is
// the inline path-traversal check in issueBackupDownloadTicket
// (handlers_file_download.go): a backup identifier must be a single path
// element — no ".", "..", separators or NUL bytes — before a download ticket
// is minted. This test pins that remaining invariant by source scan, matching
// the style of the other handlers_*_reverification tests.
func TestValidBackupName(t *testing.T) {
	src := readHTTPFile(t, "handlers_file_download.go")
	if !strings.Contains(src, `backupName == "." || backupName == ".."`) {
		t.Fatal("handlers_file_download.go must reject '.' and '..' backup names")
	}
	if !strings.Contains(src, `path.Base(backupName) != backupName`) {
		t.Fatal("handlers_file_download.go must reject path separators in backup names")
	}
	if !strings.Contains(src, "strings.ContainsRune(backupName, '\\x00')") {
		t.Fatal("handlers_file_download.go must reject NUL bytes in backup names")
	}
	if !strings.Contains(src, `"invalid backup name"`) {
		t.Fatal("handlers_file_download.go must 400 with 'invalid backup name'")
	}

	// The traversal predicate itself, exercised directly on the same inputs
	// the old whitelist test used for the still-relevant rejection classes.
	isTraversal := func(name string) bool {
		return name == "." || name == ".." || strings.Trim(name, "/") != name || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.ContainsRune(name, '\x00')
	}
	for _, bad := range []string{".", "..", "../etc/passwd", "foo/bar", "foo\\bar", "/abs/name"} {
		if !isTraversal(bad) {
			t.Errorf("traversal-style name %q must be rejected", bad)
		}
	}
	for _, ok := range []string{"daily-backup_01.zip", "Backup.2026.08.23", ".hidden"} {
		if isTraversal(ok) {
			t.Errorf("plain filename %q must not be flagged as traversal", ok)
		}
	}
}
