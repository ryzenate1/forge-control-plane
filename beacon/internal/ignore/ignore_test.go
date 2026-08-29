package ignore

import (
	"strings"
	"testing"
)

func TestGitignoreSemantics(t *testing.T) {
	list, err := LoadIgnoreReader(strings.NewReader(`
*.log
!important.log
build/
cache/**/tmp
/root-only.txt
assets/**/generated-?.png
`))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]bool{
		"server.log":                     true,
		"logs/server.log":                true,
		"important.log":                  false,
		"build/output.bin":               true,
		"nested/build/output.bin":        true,
		"cache/tmp":                      true,
		"cache/a/b/tmp":                  true,
		"root-only.txt":                  true,
		"nested/root-only.txt":           false,
		"assets/generated-a.png":         true,
		"assets/a/b/generated-1.png":     true,
		"assets/a/b/not-generated-1.png": false,
	}
	for filePath, expected := range tests {
		if actual := list.IsIgnored(filePath); actual != expected {
			t.Errorf("IsIgnored(%q) = %v, want %v", filePath, actual, expected)
		}
	}
}

func TestInvalidPatternIsRejected(t *testing.T) {
	if _, err := LoadIgnoreReader(strings.NewReader("[unterminated\n")); err == nil {
		t.Fatal("invalid pattern was accepted")
	}
}
