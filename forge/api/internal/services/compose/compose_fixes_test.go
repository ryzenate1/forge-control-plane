package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: this file used to pin three helpers that no longer exist:
// ValidateHostMountWithAllowlist (the admin + allowlist host-mount predicate),
// isEnvFileEmpty / the FORGE_ENV_FILE_STRICT gate, and isComposePathTraversal.
// The refactor dropped the allowlist model entirely: host mounts are now only
// reported as warnings by checkVolumesSecurity, while docker.sock, /proc and
// /sys stay hard errors. env_file is no longer inspected at all, and the
// path-escape guard moved inline into readComposeFromDir. The surviving
// behaviour is pinned below.

// ---------- Volume security ----------

func issuesForVolumes(t *testing.T, volumes ...string) []ValidationIssue {
	t.Helper()
	raw := make([]interface{}, 0, len(volumes))
	for _, v := range volumes {
		raw = append(raw, v)
	}
	var issues []ValidationIssue
	checkVolumesSecurity("app", raw, &issues)
	return issues
}

func findIssue(issues []ValidationIssue, severity string) *ValidationIssue {
	for i := range issues {
		if issues[i].Severity == severity {
			return &issues[i]
		}
	}
	return nil
}

func TestCheckVolumesSecurity_DockerSockAndKernelPathsAreErrors(t *testing.T) {
	cases := []struct {
		name    string
		volume  string
		message string
	}{
		{"docker.sock", "/var/run/docker.sock:/var/run/docker.sock", "docker.sock"},
		{"proc", "/proc:/host-proc", "/proc"},
		{"sys", "/sys/firmware:/host-sys", "/sys"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := issuesForVolumes(t, tc.volume)
			issue := findIssue(issues, "error")
			require.NotNil(t, issue, "expected a hard error for %s", tc.volume)
			assert.Contains(t, issue.Message, tc.message)
			assert.Contains(t, issue.Field, "services.app.volumes")
		})
	}
}

func TestCheckVolumesSecurity_SensitiveHostPathIsWarningOnly(t *testing.T) {
	// The admin/allowlist gate is gone: a sensitive host path is surfaced as a
	// warning and nothing more, regardless of who declares it.
	issues := issuesForVolumes(t, "/etc:/host-etc")
	err := findIssue(issues, "error")
	assert.Nil(t, err, "sensitive host paths must no longer be hard errors")
	warn := findIssue(issues, "warning")
	require.NotNil(t, warn)
	assert.Contains(t, warn.Message, "may expose sensitive host files")

	// A map-form volume is handled the same way.
	var mapped []ValidationIssue
	checkVolumesSecurity("app", []interface{}{map[string]interface{}{"source": "/home/alice", "target": "/mnt"}}, &mapped)
	require.NotNil(t, findIssue(mapped, "warning"))
}

func TestCheckVolumesSecurity_NonSensitivePathsPass(t *testing.T) {
	for _, vol := range []string{"/data:/data", "/srv/game-panel/volumes:/vol", "named-vol:/var/lib/mysql", "./relative:/app"} {
		t.Run(vol, func(t *testing.T) {
			assert.Empty(t, issuesForVolumes(t, vol))
		})
	}
}

func TestCheckVolumesSecurity_NilAndMalformedInput(t *testing.T) {
	var issues []ValidationIssue
	checkVolumesSecurity("app", nil, &issues)
	assert.Empty(t, issues)

	issues = nil
	checkVolumesSecurity("app", "not-a-list", &issues)
	assert.Empty(t, issues)

	issues = nil
	checkVolumesSecurity("app", []interface{}{"", map[string]interface{}{"target": "/mnt"}}, &issues)
	assert.Empty(t, issues)
}

func TestIsSensitiveHostPath(t *testing.T) {
	sensitive := []string{"/", "/etc", "/etc/passwd", "/home", "/home/user", "/root", "/root/.ssh"}
	for _, p := range sensitive {
		assert.True(t, isSensitiveHostPath(p), "%s should be sensitive", p)
	}
	safe := []string{"/data", "/var/log", "/srv/game-panel/volumes", "etc", "/etc-configs"}
	for _, p := range safe {
		assert.False(t, isSensitiveHostPath(p), "%s should not be sensitive", p)
	}
}

func TestValidateCompose_HostMountSeverityMapping(t *testing.T) {
	svc := &Service{}

	yamlSensitiveMount := `
services:
  app:
    image: nginx
    volumes:
      - /etc:/host
`
	result := svc.ValidateCompose([]byte(yamlSensitiveMount), "")
	assert.True(t, result.Valid, "a sensitive host mount warns but does not invalidate")
	require.NotEmpty(t, result.Warnings)

	yamlDockerSock := `
services:
  app:
    image: nginx
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
`
	result = svc.ValidateCompose([]byte(yamlDockerSock), "")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0].Message, "docker.sock")
}

// ---------- env_file is no longer inspected ----------

func TestEnvFileIsIgnored(t *testing.T) {
	svc := &Service{}

	for _, doc := range []string{
		"services:\n  app:\n    image: nginx\n    env_file:\n      - .env\n",
		"services:\n  app:\n    image: nginx\n    env_file: .env\n",
	} {
		_, err := svc.ParseComposeYAML([]byte(doc), "", nil)
		require.NoError(t, err, "env_file must not be rejected any more")
		result := svc.ValidateCompose([]byte(doc), "")
		assert.True(t, result.Valid)
		for _, issue := range append(append([]ValidationError{}, result.Errors...), result.Warnings...) {
			assert.NotContains(t, strings.ToLower(issue.Message), "env_file")
		}
	}
}

// ---------- Path-escape guard (replaces isComposePathTraversal) ----------

func TestReadComposeFromDir_RejectsParentTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"../compose.yml", "sub/../../escape.yml", ".."} {
		_, err := readComposeFromDir(dir, p)
		require.Error(t, err, "%s must be rejected", p)
		assert.Contains(t, err.Error(), "invalid compose path")
	}
}

func TestReadComposeFromDir_RejectsSymlinkEscape(t *testing.T) {
	dir := realTempDir(t)
	outside := realTempDir(t)
	target := filepath.Join(outside, "compose.yml")
	require.NoError(t, os.WriteFile(target, []byte("services:\n  app:\n    image: nginx\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "linked")))

	_, err := readComposeFromDir(dir, filepath.Join("linked", "compose.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes working directory")
}

func TestReadLimitedComposeFile_RejectsSymlink(t *testing.T) {
	dir := realTempDir(t)
	real := filepath.Join(dir, "base.yml")
	require.NoError(t, os.WriteFile(real, []byte("services:\n  app:\n    image: nginx\n"), 0o644))
	link := filepath.Join(dir, "compose.yml")
	require.NoError(t, os.Symlink(real, link))

	_, err := readLimitedComposeFile(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regular non-symlink")

	_, err = readLimitedComposeFile(filepath.Join(dir, "missing.yml"))
	require.Error(t, err)
}

func TestReadComposeFromDir_ReadsInTreeComposeFile(t *testing.T) {
	// Temp dirs live behind a symlinked /var -> /private/var on macOS, and the
	// guard compares EvalSymlinks output against the raw dir, so resolve first.
	dir := realTempDir(t)
	body := "services:\n  app:\n    image: nginx\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644))

	// Explicit relative path.
	got, err := readComposeFromDir(dir, "compose.yaml")
	require.NoError(t, err)
	assert.Equal(t, body, got)

	// Discovery walk finds it when no path is given.
	got, err = readComposeFromDir(dir, "")
	require.NoError(t, err)
	assert.Equal(t, body, got)

	empty := realTempDir(t)
	_, err = readComposeFromDir(empty, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no compose file found")
}

func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestIsComposePathTraversalPredicateStillReferenced(t *testing.T) {
	// The standalone helper is gone; the same intent now lives inline in
	// gitops.go, so pin the guard text to catch an accidental removal.
	src, err := os.ReadFile("gitops.go")
	require.NoError(t, err)
	assert.Contains(t, string(src), `strings.Contains(composePath, "..")`)
	assert.Contains(t, string(src), "compose symlink escapes working directory")
}

// ---------- Unchanged helpers ----------

func TestComposeDeleteWithOptions_Exists(t *testing.T) {
	// Verify method signatures exist; DB-dependent path is covered by integration tests.
	t.Skip("skip DB-dependent delete test in unit suite")
}

func TestGitOps_CreateBeforeUpdate_FreshID(t *testing.T) {
	// Simulate DeployFromGit existence check: fresh ID should trigger Create, not Update
	store := &mockStoreForFixes{stacks: make(map[string]struct{})}
	freshID := "cps-fresh12345"
	shouldFail := false
	if _, ok := store.stacks[freshID]; !ok {
		shouldFail = true
	}
	assert.True(t, shouldFail, "fresh ID should not exist")
	store.stacks[freshID] = struct{}{}
	_, ok := store.stacks[freshID]
	assert.True(t, ok, "after create, ID should exist")
}

type mockStoreForFixes struct {
	stacks map[string]struct{}
}
