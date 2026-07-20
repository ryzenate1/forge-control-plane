package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid github https", "https://github.com/user/repo.git", false},
		{"valid gitlab https", "https://gitlab.com/user/repo.git", false},
		{"valid bitbucket https", "https://bitbucket.org/user/repo.git", false},
		{"valid gitea https", "https://gitea.com/user/repo.git", false},
		{"valid codeberg https", "https://codeberg.org/user/repo.git", false},
		{"valid github http", "http://github.com/user/repo", false},
		{"valid without .git", "https://github.com/user/repo", false},
		{"valid subdomain", "https://git.internal.github.com/user/repo.git", false},
		{"empty url", "", true},
		{"ssh url", "git@github.com:user/repo.git", true},
		{"not in allowlist", "https://evil.com/user/repo.git", true},
		{"relative path traversal", "https://github.com/../../../etc", true},
		{"semicolon injection", "https://github.com/user/repo;rm -rf /", true},
		{"backtick injection", "https://github.com/`evil`/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepoURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRepoURL(%q) error = %v, wantErr = %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBranch(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		valid  bool
	}{
		{"main", "main", true},
		{"develop", "develop", true},
		{"feature branch", "feature/my-feature", true},
		{"tag prefix", "refs/tags/v1.0.0", true},
		{"empty", "", false},
		{"parent traversal", "../danger", false},
		{"hidden dir", "feature/.git", false},
		{"space", "my branch", false},
		{"backslash", "my\\branch", false},
		{"single quote", "my'branch", false},
		{"double quote", `my"branch`, false},
		{"backtick", "my`branch", false},
		{"dollar", "my$branch", false},
		{"ampersand", "my&branch", false},
		{"pipe", "my|branch", false},
		{"semicolon", "my;branch", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateBranch(tt.branch)
			if got != tt.valid {
				t.Errorf("validateBranch(%q) = %v, want %v", tt.branch, got, tt.valid)
			}
		})
	}
}

func TestAllowedHost(t *testing.T) {
	tests := []struct {
		host    string
		allowed bool
	}{
		{"https://github.com/user/repo", true},
		{"https://gitlab.com/user/repo", true},
		{"https://bitbucket.org/user/repo", true},
		{"https://gitea.com/user/repo", true},
		{"https://codeberg.org/user/repo", true},
		{"https://sub.github.com/user/repo", true},
		{"https://evil.com/repo", false},
		{"https://github.com.evil.com/repo", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := allowedHost(tt.host)
			if got != tt.allowed {
				t.Errorf("allowedHost(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestDetectProjectType(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		pt := detectProjectType(dir)
		if pt != projectTypeUnknown {
			t.Errorf("expected projectTypeUnknown, got %s", pt)
		}
	})

	t.Run("dockerfile present", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "Dockerfile", "FROM alpine")
		pt := detectProjectType(dir)
		if pt != "dockerfile" {
			t.Errorf("expected dockerfile, got %s", pt)
		}
	})

	t.Run("compose yml present", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "compose.yml", "services:")
		pt := detectProjectType(dir)
		if pt != "compose" {
			t.Errorf("expected compose, got %s", pt)
		}
	})

	t.Run("dockerfile wins over compose", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "Dockerfile", "FROM alpine")
		writeFile(t, dir, "compose.yml", "services:")
		pt := detectProjectType(dir)
		if pt != "dockerfile" {
			t.Errorf("Dockerfile should take precedence, got %s", pt)
		}
	})

	t.Run("static via index.html", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "index.html", "<html>")
		pt := detectProjectType(dir)
		if pt != "static" {
			t.Errorf("expected static, got %s", pt)
		}
	})
}

func TestSafeClonePath(t *testing.T) {
	t.Run("valid path under git-sources", func(t *testing.T) {
		dir := os.TempDir() + "/git-sources/abc123"
		got, err := safeClonePath(dir)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got == "" {
			t.Error("expected non-empty path")
		}
	})

	t.Run("rejects path outside git-sources", func(t *testing.T) {
		dir := os.TempDir() + "/evil-path"
		_, err := safeClonePath(dir)
		if err == nil {
			t.Error("expected error for path outside git-sources")
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		base := os.TempDir()
		dir := base + "/git-sources/../../../etc"
		_, err := safeClonePath(dir)
		if err == nil {
			t.Error("expected error for traversal path")
		}
	})
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fp := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(fp), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fp, []byte(content), 0o640); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
