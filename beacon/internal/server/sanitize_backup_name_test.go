package server

import (
	"strings"
	"testing"
)

// TestSanitizeBackupName guards the control-plane-supplied backup name
// contract: names accepted here become archive filenames on the node, so
// path separators, traversal sequences, and control characters must never
// pass. The canonical form always carries the .zip suffix; an empty result
// means the name was rejected.
func TestSanitizeBackupName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"daily-backup_01.zip", "daily-backup_01.zip"},
		{"Backup.2026.08.23", "Backup.2026.08.23.zip"},
		{"../etc/passwd", ""},
		{"foo/bar", ""},
		{"foo\\bar", ""},
		{".hidden", ""},
		{"..dots", ""},
		{"a..b", ""},
		{"with space", ""},
		{"with$shell", ""},
		{"quote'", ""},
		{strings.Repeat("a", 101), ""},
		{strings.Repeat("a", 100), strings.Repeat("a", 100)},
	}
	for _, tc := range cases {
		got, ok := normalizeBackupName(tc.in)
		if !ok {
			got = ""
		}
		if got != tc.want {
			t.Errorf("normalizeBackupName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
