package store

import (
	"os"
	"sort"
	"testing"
)

// Temporary audit helper: prints the runner's canonical apply order so the
// ordering claim can be checked against a shell-computed order. Deleted at the
// end of the audit.
func TestZZTmpPrintRunnerOrder(t *testing.T) {
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && e.Name() != "" && len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".sql" {
			names = append(names, e.Name())
		}
	}
	got := append([]string(nil), names...)
	sortMigrationFiles(got)
	lex := append([]string(nil), names...)
	sort.Strings(lex)
	for i := range got {
		if got[i] != lex[i] {
			t.Logf("ORDER-DIFF idx=%d runner=%s lex=%s", i, got[i], lex[i])
		}
	}
	f, err := os.Create("/tmp/go_agent_go_order.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, n := range got {
		if _, err := f.WriteString(n + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}
