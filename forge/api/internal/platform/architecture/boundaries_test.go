package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Platform is the dependency-free kernel. Capability modules may depend on
// it, but the kernel must never depend on modules, transports, stores, or
// legacy services.
func TestPlatformDependencyDirection(t *testing.T) {
	root := filepath.Clean("..")
	forbidden := []string{
		"gamepanel/forge/internal/modules/",
		"gamepanel/forge/internal/http",
		"gamepanel/forge/internal/services/",
		"gamepanel/forge/internal/store",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			for _, prefix := range forbidden {
				if strings.HasPrefix(value, prefix) {
					t.Errorf("platform package %s imports forbidden dependency %s", path, value)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
