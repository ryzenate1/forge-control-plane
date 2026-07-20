package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBuildRequestRejectsEscapeAndMissingDockerfile(t *testing.T) {
	root := t.TempDir()
	if _, _, err := validateBuildRequest(BuildRequest{ContextDir: root, DockerfilePath: "../Dockerfile", ImageTag: "forge-app:test"}); err == nil {
		t.Fatal("expected dockerfile escape to be rejected")
	}
	if _, _, err := validateBuildRequest(BuildRequest{ContextDir: root, ImageTag: "forge-app:test"}); err == nil {
		t.Fatal("expected missing Dockerfile to be rejected")
	}
}

func TestBuildContextArchiveExcludesGitAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := buildContextArchive(root); err == nil {
		t.Fatal("expected symlink to be rejected")
	}
}
