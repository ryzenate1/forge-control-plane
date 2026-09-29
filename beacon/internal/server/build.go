package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gamepanel/beacon/internal/serverid"
	"github.com/google/uuid"
)

type buildJob struct {
	id             string
	workspaceID    string
	cmd            *exec.Cmd
	logBuf         bytes.Buffer
	logCh          chan string
	cancel         context.CancelFunc
	status         string
	exitCode       int
	imageRef       string
	startedAt      time.Time
	truncated      bool
	log            *buildLogWriter
	credentialText []string
	mu             sync.RWMutex
}

// maxBuildJobDuration bounds a single build. Without it a `docker buildx` that
// stalls pins a job, its log buffer and its child process until the 12h reaper
// happens to run.
const maxBuildJobDuration = 2 * time.Hour

// buildLogWriter turns the child's combined output into complete, redacted
// lines in the job buffer. It exists so a build never uses cmd.StdoutPipe: a
// pipe must be drained before Wait returns or the tail of the log is lost, and
// draining it in a detached goroutine races with the status write.
type buildLogWriter struct {
	job     *buildJob
	mu      sync.Mutex
	partial string
}

func (w *buildLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.partial += string(p)
	var complete string
	if idx := strings.LastIndex(w.partial, "\n"); idx >= 0 {
		complete = w.partial[:idx]
		w.partial = w.partial[idx+1:]
	}
	w.mu.Unlock()
	if complete != "" {
		for _, line := range strings.Split(complete, "\n") {
			w.job.appendBuildLine(line)
		}
	}
	return len(p), nil
}

// flush writes a trailing line that arrived without a newline; a child can exit
// with a partially written final line.
func (w *buildLogWriter) flush() {
	w.mu.Lock()
	rest := w.partial
	w.partial = ""
	w.mu.Unlock()
	if rest != "" {
		w.job.appendBuildLine(rest)
	}
}

func (j *buildJob) appendBuildLine(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, pattern := range j.credentialText {
		if pattern != "" {
			line = strings.ReplaceAll(line, pattern, "****")
		}
	}
	if j.logBuf.Len()+len(line)+1 < maxLogBufferSize {
		_, _ = fmt.Fprintln(&j.logBuf, line)
		return
	}
	if !j.truncated {
		_, _ = fmt.Fprintln(&j.logBuf, "[LOG TRUNCATED: exceeded 100MB limit]")
		j.truncated = true
	}
}

// currentStatus returns the job status under the job lock. Reading job.status
// directly from a handler races with the goroutine that records the exit.
func (j *buildJob) currentStatus() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

// snapshotLog copies the buffered log. The buffer is only ever touched under
// the job lock, so callers must never keep a view of it past the lock.
func (j *buildJob) snapshotLog() []byte {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return append([]byte(nil), j.logBuf.Bytes()...)
}

func (j *buildJob) logLength() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.logBuf.Len()
}

// readLogFrom returns a copy of everything appended after offset.
func (j *buildJob) readLogFrom(offset int) []byte {
	j.mu.RLock()
	defer j.mu.RUnlock()
	buf := j.logBuf.Bytes()
	if offset >= len(buf) {
		return nil
	}
	return append([]byte(nil), buf[offset:]...)
}

type buildManager struct {
	mu     sync.RWMutex
	active map[string]*buildJob
}

const maxLogBufferSize = 100 * 1024 * 1024

type dockerfileBuildRequest struct {
	WorkspaceID        string   `json:"workspaceId"`
	SourceDir          string   `json:"sourceDir"` // deprecated: use workspaceId
	Dockerfile         string   `json:"dockerfile"`
	ImageName          string   `json:"imageName"`
	BuildArgs          []string `json:"buildArgs"`
	Labels             []string `json:"labels"`
	Tags               []string `json:"tags"`
	NoCache            bool     `json:"noCache"`
	SecretArgs         []string `json:"secretArgs,omitempty"`
	MaxCPU             int      `json:"maxCpu,omitempty"`
	MaxMemoryMB        int      `json:"maxMemoryMb,omitempty"`
	MaxLogBytes        int      `json:"maxLogBytes,omitempty"`
	CredentialPatterns []string `json:"credentialPatterns,omitempty"`
	TenantID           string   `json:"tenantId,omitempty"`
}

type nixpacksBuildRequest struct {
	WorkspaceID        string   `json:"workspaceId"`
	SourceDir          string   `json:"sourceDir"` // deprecated: use workspaceId
	ImageName          string   `json:"imageName"`
	BuildArgs          []string `json:"buildArgs"`
	Tags               []string `json:"tags"`
	NoCache            bool     `json:"noCache"`
	SecretArgs         []string `json:"secretArgs,omitempty"`
	MaxCPU             int      `json:"maxCpu,omitempty"`
	MaxMemoryMB        int      `json:"maxMemoryMb,omitempty"`
	MaxLogBytes        int      `json:"maxLogBytes,omitempty"`
	CredentialPatterns []string `json:"credentialPatterns,omitempty"`
	TenantID           string   `json:"tenantId,omitempty"`
}

func (s *Server) handleDockerfileBuild(w http.ResponseWriter, r *http.Request) {
	if _, err := exec.LookPath("docker"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "docker build CLI is unavailable"})
		return
	}
	var req dockerfileBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sourceDir, err := s.resolveWorkspace(req.WorkspaceID, req.SourceDir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workspace: " + err.Error()})
		return
	}

	if _, err := os.Stat(sourceDir); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("source directory not found: %v", err)})
		return
	}

	tid := req.TenantID
	if tid == "" {
		tid = "_system"
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("forge/%s/build-%d", tid, time.Now().UnixNano())
	}

	dockerfile := req.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(sourceDir, "Dockerfile")
	} else if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(sourceDir, filepath.Clean(dockerfile))
	}
	safeDockerfile, safeErr := safePath(dockerfile, sourceDir)
	if safeErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dockerfile path escapes workspace: " + safeErr.Error()})
		return
	}
	dockerfile = safeDockerfile

	if err := validateBuildArgs(req.BuildArgs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid build arg: " + err.Error()})
		return
	}
	if err := validateBuildArgs(req.SecretArgs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid secret arg: " + err.Error()})
		return
	}
	if err := validateBuildLabels(req.Labels); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid label: " + err.Error()})
		return
	}
	if err := validateImageTags(append(append([]string{}, req.Tags...), imageName)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid image tag: " + err.Error()})
		return
	}

	args := []string{"buildx", "build", "-f", dockerfile}

	if req.NoCache {
		args = append(args, "--no-cache")
	}

	for _, arg := range req.BuildArgs {
		args = append(args, "--build-arg", arg)
	}

	for _, secret := range req.SecretArgs {
		args = append(args, "--secret", secret)
	}

	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}

	for _, tag := range req.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageName)

	if req.MaxCPU > 0 {
		args = append(args, "--cpu-shares", fmt.Sprintf("%d", req.MaxCPU*1024/100))
	}
	if req.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", req.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", req.MaxMemoryMB))
	}
	args = append(args, "--shm-size", "256m")
	args = append(args, sourceDir)

	job := s.builds.startBuild(r.Context(), imageName, req.WorkspaceID, "docker", req.CredentialPatterns, args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":          job.id,
		"imageName":   imageName,
		"workspaceId": req.WorkspaceID,
		"status":      job.currentStatus(),
	})
}

func (s *Server) resolveWorkspace(workspaceID, legacySourceDir string) (string, error) {
	if workspaceID != "" {
		cloneBase := filepath.Join(s.dataDir, gitCloneDir)
		sourceDir := filepath.Join(cloneBase, workspaceID)
		return safePath(sourceDir, cloneBase)
	}
	const serverPrefix = "server:"
	if strings.HasPrefix(legacySourceDir, serverPrefix) {
		serverID := strings.TrimPrefix(legacySourceDir, serverPrefix)
		if err := serverid.Validate(serverID); err != nil {
			return "", fmt.Errorf("invalid server workspace: %w", err)
		}
		return safePath(filepath.Join(s.dataDir, serverID), s.dataDir)
	}
	return "", fmt.Errorf("workspace must be a managed clone or server root")
}

func (s *Server) handleNixpacksBuild(w http.ResponseWriter, r *http.Request) {
	if _, err := exec.LookPath("nixpacks"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nixpacks build CLI is unavailable"})
		return
	}
	var req nixpacksBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sourceDir, err := s.resolveWorkspace(req.WorkspaceID, req.SourceDir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workspace: " + err.Error()})
		return
	}

	tid := req.TenantID
	if tid == "" {
		tid = "_system"
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("forge/%s/build-%d", tid, time.Now().UnixNano())
	}

	if err := validateImageTags(append(append([]string{}, req.Tags...), imageName)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid image tag: " + err.Error()})
		return
	}
	if err := validateBuildArgs(req.BuildArgs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid build arg: " + err.Error()})
		return
	}

	args := []string{"nixpacks", "build", sourceDir, "--name", imageName}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	for _, arg := range req.BuildArgs {
		args = append(args, "--build-env", arg)
	}
	for _, tag := range req.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageName)

	job := s.builds.startBuild(r.Context(), imageName, req.WorkspaceID, "nixpacks", req.CredentialPatterns, args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":          job.id,
		"imageName":   imageName,
		"workspaceId": req.WorkspaceID,
		"status":      job.currentStatus(),
	})
}

func (s *Server) handleBuildLogs(w http.ResponseWriter, r *http.Request) {
	buildID := r.URL.Query().Get("id")
	if buildID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id query parameter required"})
		return
	}

	s.builds.mu.RLock()
	job, ok := s.builds.active[buildID]
	s.builds.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "build not found"})
		return
	}

	follow := r.URL.Query().Get("follow") == "true"

	if job.isTerminal() || !follow {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(job.snapshotLog())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	offset := job.logLength()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	// A build that produces no output for minutes is indistinguishable from a
	// dead stream to every proxy in between, so say something periodically
	// instead of letting the connection be silently reaped.
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
			if job.isTerminal() {
				_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", job.currentStatus())
				flusher.Flush()
				return
			}
		case <-ticker.C:
			if chunk := job.readLogFrom(offset); len(chunk) > 0 {
				offset += len(chunk)
				for _, line := range strings.Split(string(chunk), "\n") {
					if line != "" {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
						flusher.Flush()
					}
				}
			}

			if job.isTerminal() {
				_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", job.currentStatus())
				flusher.Flush()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleBuildCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	s.builds.mu.RLock()
	job, ok := s.builds.active[body.ID]
	s.builds.mu.RUnlock()

	if !ok || job.isTerminal() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active build not found"})
		return
	}

	job.cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (m *buildManager) startBuild(ctx context.Context, imageRef, workspaceID string, command string, credPatterns []string, args ...string) *buildJob {
	// A build must not be tied to the request that started it: r.Context() is
	// cancelled the instant this handler writes its 202 response, which killed
	// every accepted build while the panel still believed it was running. The
	// job is bounded by maxBuildJobDuration instead, cancelled explicitly
	// through POST /build/cancel, and reaped if it outlives both.
	buildCtx, cancel := context.WithTimeout(context.Background(), maxBuildJobDuration)
	cmd := exec.CommandContext(buildCtx, command, args...)
	cmd.SysProcAttr = getSysProcAttr()
	buildHome, homeErr := os.MkdirTemp("", "beacon-build-home-*")
	if homeErr == nil {
		_ = os.Chmod(buildHome, 0o700)
	}
	cmd.Env = buildEnvironment(buildHome)

	job := &buildJob{
		id:             "build-" + uuid.NewString(),
		workspaceID:    workspaceID,
		cmd:            cmd,
		cancel:         cancel,
		status:         "running",
		imageRef:       imageRef,
		startedAt:      time.Now(),
		logCh:          make(chan string, 256),
		credentialText: append([]string(nil), credPatterns...),
	}
	job.log = &buildLogWriter{job: job}
	cmd.Stdout = job.log
	cmd.Stderr = job.log

	m.mu.Lock()
	m.active[job.id] = job
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		if buildHome != "" {
			_ = os.RemoveAll(buildHome)
		}
		job.mu.Lock()
		job.status = "failed"
		job.exitCode = -1
		_, _ = fmt.Fprintf(&job.logBuf, "ERROR: %v\n", err)
		job.mu.Unlock()
		cancel()
		return job
	}

	pid := cmd.Process.Pid
	processDone := make(chan struct{})
	go func() {
		select {
		case <-buildCtx.Done():
			if cmd.Process != nil {
				_ = killPIDGroup(cmd, pid, syscall.SIGINT)
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-timer.C:
					_ = killPIDGroup(cmd, pid, syscall.SIGKILL)
				case <-processDone:
					timer.Stop()
				}
			}
		case <-processDone:
		}
	}()

	go func() {
		// cmd.Stdout is a plain io.Writer, so exec.Cmd copies the child output in
		// its own goroutines and Wait drains them before returning: no log line is
		// lost and no reader races the status write.
		err := cmd.Wait()
		job.log.flush()
		close(processDone)
		if buildHome != "" {
			_ = os.RemoveAll(buildHome)
		}
		job.mu.Lock()
		defer job.mu.Unlock()

		if err != nil {
			if buildCtx.Err() != nil {
				if buildCtx.Err() == context.DeadlineExceeded {
					job.status = "failed"
					job.exitCode = -1
					_, _ = fmt.Fprintf(&job.logBuf, "Build timed out after %s\n", maxBuildJobDuration)
				} else {
					job.status = "canceled"
					job.exitCode = -1
					_, _ = fmt.Fprintln(&job.logBuf, "Build canceled")
				}
			} else if exitErr, ok := err.(*exec.ExitError); ok {
				job.status = "failed"
				job.exitCode = exitErr.ExitCode()
				_, _ = fmt.Fprintf(&job.logBuf, "Build failed with exit code %d\n", job.exitCode)
			} else {
				job.status = "failed"
				job.exitCode = -1
				_, _ = fmt.Fprintf(&job.logBuf, "Build error: %v\n", err)
			}
		} else {
			job.status = "succeeded"
			job.exitCode = 0
			_, _ = fmt.Fprintln(&job.logBuf, "Build succeeded")
		}
	}()

	return job
}

// validateBuildArgs rejects build/secret args that are not KEY=VALUE with a
// safe name, or that smuggle newlines, null bytes, or shell metacharacters
// into the docker CLI argv.
func validateBuildArgs(args []string) error {
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("entry %q must be KEY=VALUE", arg)
		}
		if strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("entry %q contains disallowed characters", name)
		}
		for _, r := range name {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '.' && r != '-' {
				return fmt.Errorf("invalid build arg name %q", name)
			}
		}
		if strings.HasPrefix(strings.TrimSpace(value), "-") && strings.TrimSpace(value) != "" {
			// Values that look like flags are passed as a single --build-arg
			// token (never split), so this is defense-in-depth, not the
			// primary barrier; still reject the ambiguous case.
			return fmt.Errorf("build arg %q value must not look like a flag", name)
		}
	}
	return nil
}

func validateBuildLabels(labels []string) error { return validateBuildArgs(labels) }

// validateImageTags rejects image references that could escape the intended
// repository (path traversal, absolute paths, shell metacharacters, or
// whitespace). Tags are passed as single -t tokens, never through a shell.
func validateImageTags(tags []string) error {
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if value == "" {
			return fmt.Errorf("image tag must not be empty")
		}
		if len(value) > 256 || strings.ContainsAny(value, "\x00\r\n \t;|&$`'\"*?~#(){}[]!\\") || strings.Contains(value, "..") {
			return fmt.Errorf("invalid image tag %q", tag)
		}
	}
	return nil
}

func buildEnvironment(home string) []string {
	env := []string{"DOCKER_BUILDKIT=1"}
	for _, key := range []string{"PATH", "DOCKER_HOST", "DOCKER_CONFIG", "XDG_RUNTIME_DIR", "TMPDIR", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	if home != "" {
		env = append(env, "HOME="+home)
	}
	return env
}

func (j *buildJob) isTerminal() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == "succeeded" || j.status == "failed" || j.status == "canceled"
}

func (m *buildManager) reapAbandoned() {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now()
	for id, job := range m.active {
		job.mu.RLock()
		status := job.status
		startedAt := job.startedAt
		job.mu.RUnlock()
		if status == "running" {
			if cutoff.Sub(startedAt) > 12*time.Hour {
				job.mu.Lock()
				job.status = "abandoned"
				job.exitCode = -1
				job.mu.Unlock()
				job.cancel()
				delete(m.active, id)
			}
			continue
		}
		// Terminal jobs (succeeded/failed/canceled/abandoned) are retained for
		// a bounded window so clients can poll their status, then evicted so
		// the daemon does not accumulate log buffers (up to 100MB each)
		// indefinitely.
		if cutoff.Sub(startedAt) > maxCompletedRetention {
			delete(m.active, id)
		}
	}
}

const maxCompletedRetention = time.Hour

func (s *Server) startBuildReaper(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		cleanupOrphanedClones(s.dataDir)

		for {
			select {
			case <-ticker.C:
				s.builds.reapAbandoned()
				cleanupStaleClones(s.dataDir)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func cleanupOrphanedClones(dataDir string) {
	cloneBase := filepath.Join(dataDir, gitCloneDir)
	entries, err := os.ReadDir(cloneBase)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > 24*time.Hour {
				_ = os.RemoveAll(filepath.Join(cloneBase, entry.Name()))
			}
		}
	}
}

func cleanupStaleClones(dataDir string) {
	cleanupOrphanedClones(dataDir)
}
