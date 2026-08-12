package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// NOTE: this file used to drive an exported builder contract
// (DockerfileBuilder/NixpacksBuilder returning a *BuildResult with
// Logs/ExitCode/ImageRef/Digest) plus a node-capability selection and retry
// layer (NewNodeCapabilitySelector, SelectBuildNode, svc.selectNode,
// RetryBuild) and log credential redaction (maskCredentials). The refactor
// unexported the builders (dockerfileBuilder/nixpacksBuilder), changed Build
// to return ([]string, error) of raw command output, narrowed
// validateBuildContext to just the source directory (platform is defaulted,
// not validated), and deleted the selector/retry/masking helpers outright.
// The mock store / mock node store / mock daemon client seams that fed those
// tests are gone too — Service now takes the concrete *store.Store and
// *daemon.Client. What still exists is pinned below.

// ---------- Helpers ----------

// dockerAvailable reports whether a docker daemon can be reached, so the
// tests that actually shell out to `docker buildx` skip instead of failing on
// machines without docker.
func dockerAvailable(t *testing.T) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lines, err := runBuildCommand(ctx, "docker", []string{"version"})
	if err != nil {
		t.Logf("docker unavailable, skipping docker-executing test: %v (%d output lines)", err, len(lines))
		return false
	}
	return true
}

// ---------- ID generation ----------

func TestGenerateBuildID(t *testing.T) {
	id1 := GenerateBuildID()
	id2 := GenerateBuildID()

	if id1 == id2 {
		t.Fatal("generated build IDs should be unique")
	}
	if len(id1) != 14 {
		t.Fatalf("expected build ID length 14, got %d: %s", len(id1), id1)
	}

	for _, c := range id1 {
		if !strings.ContainsRune(base36Alphabet, c) {
			t.Fatalf("build ID contains invalid character: %c", c)
		}
	}

	id3 := GenerateBuildID()
	if id3 <= id2 {
		t.Fatal("build IDs should be lexicographically sortable (monotonically increasing)")
	}
}

func TestGenerateBuildID_Sortability(t *testing.T) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = GenerateBuildID()
		time.Sleep(time.Millisecond)
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("build IDs not sortable: %s <= %s at index %d", ids[i], ids[i-1], i)
		}
	}
}

func TestEncodeBase36(t *testing.T) {
	tests := []struct {
		value    uint64
		minWidth int
		expected string
	}{
		{0, 1, "0"},
		{0, 8, "00000000"},
		{10, 1, "a"},
		{35, 1, "z"},
		{36, 1, "10"},
		{36, 4, "0010"},
		{1295, 1, "zz"},
	}

	for _, tt := range tests {
		result := encodeBase36(tt.value, tt.minWidth)
		if result != tt.expected {
			t.Errorf("encodeBase36(%d, %d) = %q, want %q", tt.value, tt.minWidth, result, tt.expected)
		}
	}
}

func TestDuplicateBuild(t *testing.T) {
	id1 := GenerateBuildID()
	id2 := GenerateBuildID()
	if id1 == id2 {
		t.Fatal("duplicate build IDs generated")
	}

	// Check sortable property
	if id1 > id2 {
		t.Logf("id1=%s, id2=%s (id1 > id2 means id2 was generated later but has lower value)", id1, id2)
		// Generate a third one to verify monotonic increase
		id3 := GenerateBuildID()
		if id3 <= id2 {
			t.Fatal("build IDs should be monotonically increasing")
		}
	}
}

// ---------- Builder detection ----------

func TestDockerfileBuilder_Detect(t *testing.T) {
	builder := dockerfileBuilder{}

	tmpDir := t.TempDir()
	if builder.Detect(tmpDir) {
		t.Fatal("should not detect Dockerfile in empty directory")
	}

	err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM alpine\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	if !builder.Detect(tmpDir) {
		t.Fatal("should detect Dockerfile when present")
	}
}

func TestNixpacksBuilder_Detect(t *testing.T) {
	builder := nixpacksBuilder{}

	tmpDir := t.TempDir()

	if builder.Detect(tmpDir) {
		t.Fatal("should not detect project in empty directory")
	}

	err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"test"}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	if !builder.Detect(tmpDir) {
		t.Fatal("should detect Node.js project with package.json")
	}
}

func TestNixpacksBuilder_Detect_DockerfilePriority(t *testing.T) {
	builder := nixpacksBuilder{}

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM alpine\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{}`), 0644)

	if builder.Detect(tmpDir) {
		t.Fatal("Nixpacks builder should yield to Dockerfile builder when Dockerfile present")
	}
}

func TestServiceDetect(t *testing.T) {
	svc := NewService(nil, nil, nil)

	if _, err := svc.Detect("   "); err == nil {
		t.Fatal("expected error for blank source dir")
	}

	empty := t.TempDir()
	if _, err := svc.Detect(empty); err == nil || !strings.Contains(err.Error(), "no supported builder") {
		t.Fatalf("expected 'no supported builder' error for empty dir, got %v", err)
	}

	withDockerfile := t.TempDir()
	_ = os.WriteFile(filepath.Join(withDockerfile, "Dockerfile"), []byte("FROM alpine\n"), 0644)
	if got, err := svc.Detect(withDockerfile); err != nil || got != BuilderDockerfile {
		t.Fatalf("expected dockerfile, got %q (err=%v)", got, err)
	}

	withPackageJSON := t.TempDir()
	_ = os.WriteFile(filepath.Join(withPackageJSON, "package.json"), []byte(`{"name":"test"}`), 0644)
	if got, err := svc.Detect(withPackageJSON); err != nil || got != BuilderNixpacks {
		t.Fatalf("expected nixpacks, got %q (err=%v)", got, err)
	}

	// Dockerfile wins over nixpacks indicators.
	both := t.TempDir()
	_ = os.WriteFile(filepath.Join(both, "Dockerfile"), []byte("FROM alpine\n"), 0644)
	_ = os.WriteFile(filepath.Join(both, "package.json"), []byte(`{}`), 0644)
	if got, err := svc.Detect(both); err != nil || got != BuilderDockerfile {
		t.Fatalf("expected dockerfile to take precedence, got %q (err=%v)", got, err)
	}
}

// ---------- Status handling ----------

func TestBuildStatus_Terminal(t *testing.T) {
	if IsTerminal(BuildRunning) {
		t.Fatal("running should not be terminal")
	}
	if !IsTerminal(BuildSucceeded) {
		t.Fatal("succeeded should be terminal")
	}
	if !IsTerminal(BuildFailed) {
		t.Fatal("failed should be terminal")
	}
	if !IsTerminal(BuildCanceled) {
		t.Fatal("canceled should be terminal")
	}
	if !IsTerminal(BuildAbandoned) {
		t.Fatal("abandoned should be terminal")
	}
}

func TestBuildRecord_DisplayStatus_Abandoned(t *testing.T) {
	pid := os.Getpid()
	record := &store.BuildRecord{
		Status: string(BuildRunning),
		PID:    &pid,
	}
	display := DisplayBuildStatus(record)
	if display != BuildRunning {
		t.Fatalf("expected running, got %s", display)
	}

	bogusPID := 99999999
	record.PID = &bogusPID
	display = DisplayBuildStatus(record)
	if display != BuildAbandoned {
		t.Fatalf("expected abandoned for dead PID, got %s", display)
	}
}

func TestNodeDisconnect(t *testing.T) {
	// Simulate a build where the node becomes unavailable
	// The build should be detected as abandoned
	pid := 99999999 // non-existent PID
	record := &store.BuildRecord{
		Status: string(BuildRunning),
		PID:    &pid,
	}
	display := DisplayBuildStatus(record)
	if display != BuildAbandoned {
		t.Fatalf("expected abandoned for dead PID, got %s", display)
	}
}

// ---------- Context validation ----------

func TestValidateBuildContext(t *testing.T) {
	// The current contract validates only the source directory: non-empty,
	// existing, and a directory. Platform is defaulted upstream in
	// StartBuild, never validated here, so BuildOptions no longer feeds this
	// helper at all.
	if err := validateBuildContext(""); err == nil {
		t.Fatal("expected error for empty source dir")
	}

	tmpDir := t.TempDir()
	if err := validateBuildContext(tmpDir); err != nil {
		t.Fatalf("unexpected error for valid dir: %v", err)
	}

	if err := validateBuildContext("/nonexistent-dir-12345"); err == nil {
		t.Fatal("expected error for non-existent dir")
	}

	asFile := filepath.Join(tmpDir, "plain.txt")
	_ = os.WriteFile(asFile, []byte("x"), 0644)
	if err := validateBuildContext(asFile); err == nil || !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("expected error for non-directory source, got %v", err)
	}
}

// ---------- StartBuild argument handling ----------

func TestStartBuild_InvalidBuilderType(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, err := svc.StartBuild(context.Background(), "test-source", "invalid", BuildOptions{}, nil)
	if err == nil {
		t.Fatal("expected error for invalid builder type")
	}
	if !strings.Contains(err.Error(), "unknown builder type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartBuild_BlankBuilderTypeDetectionFailure(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Empty source dir cannot be resolved to a builder, so StartBuild must
	// fail before it ever touches the (nil) store.
	_, err := svc.StartBuild(context.Background(), "test-source", "", BuildOptions{SourceDir: t.TempDir()}, nil)
	if err == nil || !strings.Contains(err.Error(), "no supported builder") {
		t.Fatalf("expected detection failure, got %v", err)
	}
}

// ---------- Docker-executing build tests ----------

func TestDockerfileBuilder_Build_Local(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker daemon unavailable")
	}
	builder := dockerfileBuilder{}
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	err := os.WriteFile(dockerfile, []byte("FROM alpine:3.21\nRUN echo hello\nCMD echo done\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logs, err := builder.Build(ctx, BuildOptions{
		SourceDir: tmpDir,
		ImageName: "forge-test-build-" + GenerateBuildID(),
	})
	if err != nil {
		// Legacy contract surfaced a result struct here; Build now returns
		// raw output lines alongside the error, so surface them in the log.
		t.Logf("build error (may be expected if the base image is unreachable): %v", err)
		t.Logf("build output: %d line(s)", len(logs))
		return
	}
	t.Logf("build succeeded, %d output line(s)", len(logs))
}

func TestDockerfileBuilder_Build_InvalidDockerfile(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker daemon unavailable")
	}
	builder := dockerfileBuilder{}
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	// Write an invalid dockerfile
	_ = os.WriteFile(dockerfile, []byte("FROM nonexistent-image-that-does-not-exist/local:tag\nRUN invalid-command\n"), 0644)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := builder.Build(ctx, BuildOptions{
		SourceDir: tmpDir,
		ImageName: "forge-test-invalid-" + GenerateBuildID(),
	})
	if err == nil {
		t.Fatal("expected error for invalid Dockerfile")
	}
	t.Logf("expected error for invalid Dockerfile: %v", err)
}

func TestDockerfileBuilder_Build_Timeout(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker daemon unavailable")
	}
	builder := dockerfileBuilder{}
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	_ = os.WriteFile(dockerfile, []byte("FROM alpine:3.21\nRUN sleep 30\n"), 0644)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := builder.Build(ctx, BuildOptions{
		SourceDir: tmpDir,
		ImageName: "forge-test-timeout-" + GenerateBuildID(),
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Log("build timed out as expected")
	} else {
		t.Logf("build error: %v", err)
	}
}

func TestDockerfileBuilder_Build_Cancellation(t *testing.T) {
	if !dockerAvailable(t) {
		t.Skip("docker daemon unavailable")
	}
	builder := dockerfileBuilder{}
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	_ = os.WriteFile(dockerfile, []byte("FROM alpine:3.21\nRUN sleep 60\nCMD sleep 300\n"), 0644)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := builder.Build(ctx, BuildOptions{
		SourceDir: tmpDir,
		ImageName: "forge-test-cancel-" + GenerateBuildID(),
	})
	if err == nil {
		t.Log("build completed before cancellation (possible if docker is fast enough)")
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Log("build was properly canceled via context")
	} else {
		t.Logf("build error: %v", err)
	}
}

func TestDockerfileBuilder_Build_RejectsMissingContext(t *testing.T) {
	builder := dockerfileBuilder{}
	// Validation happens before any docker invocation, so this never shells
	// out and is safe without a daemon.
	if _, err := builder.Build(context.Background(), BuildOptions{}); err == nil {
		t.Fatal("expected validation error for empty source dir")
	}
}

func TestNixpacksBuilder_Build_RejectsMissingContext(t *testing.T) {
	builder := nixpacksBuilder{}
	if _, err := builder.Build(context.Background(), BuildOptions{}); err == nil {
		t.Fatal("expected validation error for empty source dir")
	}
}

// ---------- Daemon-side digest helper ----------

func TestDigestFromBuildOutput(t *testing.T) {
	logs := "Step 1/3 : FROM alpine\nStep 2/3 : RUN echo hello\n => exporting to image\n => => exporting layers\n => => writing image sha256:abc123\n => => naming to docker.io/library/test:latest\n digest: sha256:def456"
	digest := daemon.DigestFromBuildOutput(logs)
	if digest != "sha256:def456" {
		t.Fatalf("expected sha256:def456, got %q", digest)
	}

	// No digest
	digest2 := daemon.DigestFromBuildOutput("no digest here")
	if digest2 != "" {
		t.Fatalf("expected empty, got %q", digest2)
	}
}

func TestDigestVerification(t *testing.T) {
	// Digest extraction lives in the daemon helper now; the local builder
	// just returns raw output lines.
	buildLog := "#5 naming to docker.io/library/test:latest done\ndigest: sha256:def789abc012\n"
	if got := daemon.DigestFromBuildOutput(buildLog); got != "sha256:def789abc012" {
		t.Fatalf("expected sha256:def789abc012, got %q", got)
	}
}
