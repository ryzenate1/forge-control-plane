package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/forge/internal/services/build"
	"gamepanel/forge/internal/services/compose"
)

// ActionExecutor runs one pipeline stage. Executors must write progress
// through logf and signal retryability requirements to the runner. The
// runner owns retry policy, timeouts and stage bookkeeping.
type ActionExecutor func(ctx context.Context, run *Run, stage *StageRun, logf func(level, format string, args ...any)) error

func (s *Service) executorFor(action Action) (ActionExecutor, bool) {
	switch action {
	case ActionPull:
		return s.execPull, true
	case ActionBuild:
		return s.execBuild, true
	case ActionDeploy:
		return s.execDeploy, true
	case ActionCompose:
		return s.execCompose, true
	case ActionHealthCheck:
		return s.execHealthCheck, true
	case ActionNotify:
		return s.execNotify, true
	case ActionApproval:
		return s.execApproval, true
	case ActionSleep:
		return s.execSleep, true
	case ActionScript:
		return s.execScript, true
	}
	return nil, false
}

func cfgString(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return strings.TrimSpace(s)
}

func cfgInt(config map[string]any, key string) int {
	f, _ := config[key].(float64)
	return int(f)
}

func cfgBool(config map[string]any, key string) bool {
	b, _ := config[key].(bool)
	return b
}

func stringSlice(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	out := []string{}
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if rawStr, ok := m[key].([]string); ok {
		out = append(out, rawStr...)
	}
	return out
}

func stringMap(m map[string]any, key string) map[string]string {
	raw, _ := m[key].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			out[k] = sv
		}
	}
	return out
}

// runLocalCommand executes a command with an explicit argv (never a shell
// string), so pipeline configs cannot escape into shell interpolation.
func runLocalCommand(ctx context.Context, name string, args []string, workdir string, logf func(level, format string, args ...any)) error {
	logf("info", "$ %s %s", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if s := strings.TrimSpace(buf.String()); s != "" {
			logf("error", "%s", s)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		logf("info", "%s", s)
	}
	return nil
}

// execPull clones or updates a git checkout in the configured workdir.
func (s *Service) execPull(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	cfg := st.Config
	repo := cfgString(cfg, "repositoryUrl")
	workdir := cfgString(cfg, "workdir")
	if repo == "" || workdir == "" {
		return errors.New("pull action requires repositoryUrl and workdir")
	}
	branch := cfgString(cfg, "branch")
	if branch == "" {
		branch = "main"
	}
	if _, err := os.Stat(filepath.Join(workdir, ".git")); err == nil {
		if err := runLocalCommand(ctx, "git", []string{"fetch", "--all", "--prune"}, workdir, logf); err != nil {
			return err
		}
		if err := runLocalCommand(ctx, "git", []string{"checkout", branch}, workdir, logf); err != nil {
			return err
		}
		return runLocalCommand(ctx, "git", []string{"reset", "--hard", "origin/" + branch}, workdir, logf)
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return fmt.Errorf("create workdir: %w", err)
	}
	return runLocalCommand(ctx, "git", []string{"clone", "--branch", branch, "--depth", "1", repo, "."}, workdir, logf)
}

// execBuild wraps the build service exactly like the /builds handler:
// StartBuild with a drained log channel, then polling until terminal.
func (s *Service) execBuild(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	if s.buildSvc == nil {
		return errors.New("build action requires BuildService; it is not configured on this API instance")
	}
	cfg := st.Config
	sourceID := cfgString(cfg, "sourceId")
	sourceDir := cfgString(cfg, "sourceDir")
	if sourceID == "" || sourceDir == "" {
		return errors.New("build action requires sourceId and sourceDir")
	}
	builderType := build.BuilderType(cfgString(cfg, "builderType"))
	if builderType == "" {
		detected, err := s.buildSvc.Detect(sourceDir)
		if err != nil {
			return fmt.Errorf("detect builder: %w", err)
		}
		builderType = detected
	}
	opts := build.BuildOptions{
		SourceDir:  sourceDir,
		Dockerfile: cfgString(cfg, "dockerfile"),
		ImageName:  cfgString(cfg, "imageName"),
		BuildArgs:  stringSlice(cfg, "buildArgs"),
		Tags:       stringSlice(cfg, "tags"),
		NoCache:    cfgBool(cfg, "noCache"),
	}
	logCh := make(chan build.BuildLogEntry, 256)
	record, err := s.buildSvc.StartBuild(ctx, sourceID, builderType, opts, logCh)
	if err != nil {
		return fmt.Errorf("start build: %w", err)
	}
	logf("info", "build %s started with %s builder", record.ID, builderType)
	go func() {
		for entry := range logCh {
			if strings.TrimSpace(entry.Line) != "" {
				logf("info", "%s", entry.Line)
			}
		}
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = s.buildSvc.CancelBuild(context.Background(), record.ID)
			return ctx.Err()
		case <-ticker.C:
			current, err := s.buildSvc.GetBuild(ctx, record.ID)
			if err != nil {
				// Poll races with the final write; tolerate transient errors.
				continue
			}
			switch build.DisplayBuildStatus(current) {
			case build.BuildSucceeded:
				logf("info", "build %s succeeded", record.ID)
				return nil
			case build.BuildFailed:
				return fmt.Errorf("build %s failed", record.ID)
			case build.BuildCanceled:
				return errors.New("build was canceled")
			case build.BuildAbandoned:
				return errors.New("build was abandoned")
			}
		}
	}
}

// execCompose deploys a compose project; when repositoryUrl is present the
// repo is cloned into a temp workdir first (mirrors the compose git-deploy
// handler flow).
func (s *Service) execCompose(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	if s.composeSvc == nil {
		return errors.New("compose action requires the compose service; it is not configured")
	}
	cfg := st.Config
	req := compose.DeployComposeRequest{
		UserID:      run.RequestedBy,
		Name:        cfgString(cfg, "name"),
		NodeID:      cfgString(cfg, "nodeId"),
		EnvVars:     stringMap(cfg, "envVars"),
		MemoryMB:    int64(cfgInt(cfg, "memoryMb")),
		CPUShares:   int64(cfgInt(cfg, "cpuShares")),
		DiskMB:      int64(cfgInt(cfg, "diskMb")),
		ComposeYAML: cfgString(cfg, "composeYaml"),
	}
	repo := cfgString(cfg, "repositoryUrl")
	if repo != "" {
		tmp, err := os.MkdirTemp("", "pipeline-compose-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmp)
		branch := cfgString(cfg, "branch")
		if branch == "" {
			branch = "main"
		}
		if err := runLocalCommand(ctx, "git", []string{"clone", "--branch", branch, "--depth", "1", repo, "."}, tmp, logf); err != nil {
			return err
		}
		content, err := readComposeForPipeline(tmp, cfgString(cfg, "composePath"))
		if err != nil {
			return err
		}
		req.ComposeYAML = content
	}
	if req.ComposeYAML == "" {
		return errors.New("compose action requires composeYaml or repositoryUrl")
	}
	if req.Name == "" {
		suffix := strings.ReplaceAll(run.ID, "-", "")
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
		req.Name = "pipeline-" + suffix
	}
	parsed := s.composeSvc.Validate([]byte(req.ComposeYAML), "")
	if parsed != nil && !parsed.Valid {
		logf("warn", "compose validation reported %d errors; deploying anyway", len(parsed.Errors))
	}
	stack, err := s.composeSvc.DeployComposeStack(ctx, req)
	if err != nil {
		return fmt.Errorf("deploy compose stack: %w", err)
	}
	logf("info", "compose stack %s deployed", stack.Name)
	return nil
}

// execDeploy triggers an existing deployment record through the deployment
// service so its step/healthgate/rollback machinery runs under the pipeline.
func (s *Service) execDeploy(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	if s.deploySvc == nil {
		return errors.New("deploy action requires the deployment service; it is not configured")
	}
	deploymentID := cfgString(st.Config, "deploymentId")
	if deploymentID == "" {
		return errors.New("deploy action requires deploymentId")
	}
	logf("info", "executing deployment %s", deploymentID)
	if err := s.deploySvc.ExecuteDeployment(ctx, deploymentID); err != nil {
		return fmt.Errorf("deploy execution failed: %w", err)
	}
	dep, err := s.deploySvc.GetDeployment(ctx, deploymentID)
	if err == nil {
		logf("info", "deployment %s finished: %s", deploymentID, dep.Status)
	}
	return nil
}

// execHealthCheck polls an HTTP endpoint until it sees threshold consecutive
// success responses or the timeout elapses — the deployment health-gate
// semantics.
func (s *Service) execHealthCheck(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	cfg := st.Config
	target := cfgString(cfg, "url")
	if target == "" {
		host := cfgString(cfg, "host")
		if host == "" {
			host = "localhost"
		}
		port := cfgInt(cfg, "port")
		if port <= 0 {
			port = 80
		}
		path := cfgString(cfg, "path")
		if path == "" {
			path = "/"
		}
		target = fmt.Sprintf("http://%s:%d%s", host, port, path)
	}
	threshold := cfgInt(cfg, "threshold")
	if threshold <= 0 {
		threshold = 1
	}
	interval := cfgInt(cfg, "intervalSec")
	if interval <= 0 {
		interval = 5
	}
	timeoutSec := cfgInt(cfg, "timeoutSec")
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	consecutive := 0
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("health check %s timed out after needing %d consecutive successes", target, threshold)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return fmt.Errorf("health check request: %w", err)
		}
		status := 0
		probeErr := ""
		resp, err := client.Do(req)
		if err != nil {
			probeErr = err.Error()
			consecutive = 0
		} else {
			status = resp.StatusCode
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			_ = resp.Body.Close()
			if status >= 200 && status < 400 {
				consecutive++
				logf("info", "health probe %d/%d (%d)", consecutive, threshold, status)
				if consecutive >= threshold {
					return nil
				}
			} else {
				consecutive = 0
				logf("warn", "health probe %d: %s", status, strings.TrimSpace(string(body)))
			}
		}
		if probeErr != "" {
			logf("warn", "health probe failed: %s", probeErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// execNotify fires an outbound webhook. {{runId}}/{{pipelineId}} tokens are
// substituted in the body.
func (s *Service) execNotify(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	cfg := st.Config
	url := cfgString(cfg, "url")
	if url == "" {
		return errors.New("notify action requires url")
	}
	method := strings.ToUpper(cfgString(cfg, "method"))
	if method == "" {
		method = http.MethodPost
	}
	body := cfgString(cfg, "body")
	body = strings.ReplaceAll(body, "{{runId}}", run.ID)
	body = strings.ReplaceAll(body, "{{pipelineId}}", run.PipelineID)
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify request: %w", err)
	}
	setHeaders(req, cfg, run)
	if _, ok := cfg["headers"]; !ok {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("notify delivery failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("notify %s %s returned %d: %s", method, url, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	logf("info", "notify %s %s -> %d", method, url, resp.StatusCode)
	return nil
}

func setHeaders(req *http.Request, cfg map[string]any, run *Run) {
	raw, ok := cfg["headers"].(map[string]any)
	if !ok {
		return
	}
	for k, v := range raw {
		vs, _ := v.(string)
		vs = strings.ReplaceAll(vs, "{{runId}}", run.ID)
		req.Header.Set(k, vs)
	}
}

// execSleep waits for the configured duration, honouring cancellation.
func (s *Service) execSleep(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	secs := cfgInt(st.Config, "durationSec")
	if secs <= 0 {
		secs = 1
	}
	logf("info", "sleeping for %d seconds", secs)
	select {
	case <-time.After(time.Duration(secs) * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// execScript runs a command inside a container through the daemon admin exec
// endpoint — scripts never execute on the control plane itself.
func (s *Service) execScript(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	if s.daemonCli == nil || s.store == nil {
		return errors.New("script action requires the daemon client and postgres")
	}
	cfg := st.Config
	nodeID := cfgString(cfg, "nodeId")
	containerID := cfgString(cfg, "containerId")
	command := cfgString(cfg, "command")
	if nodeID == "" || containerID == "" || command == "" {
		return errors.New("script action requires nodeId, containerId and command")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("script node lookup: %w", err)
	}
	token, err := s.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("script node credential: %w", err)
	}
	argv := []string{"sh", "-c", command}
	if raw, ok := cfg["argv"].([]any); ok && len(raw) > 0 {
		argv = []string{}
		for _, v := range raw {
			if sv, ok := v.(string); ok {
				argv = append(argv, sv)
			}
		}
	}
	logf("info", "exec in container %s: %s", containerID, strings.Join(argv, " "))
	out, err := s.daemonCli.AdminContainerExec(ctx, node.BaseURL, token, containerID, argv, false)
	if err != nil {
		return fmt.Errorf("container exec: %w", err)
	}
	var payload struct {
		ExitCode int    `json:"exitCode"`
		Output   string `json:"output"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(out, &payload); err == nil {
		if payload.Output != "" {
			logf("info", "%s", payload.Output)
		}
		if payload.ExitCode != 0 {
			msg := payload.Error
			if msg == "" {
				msg = payload.Output
			}
			return fmt.Errorf("container exec: exit code %d: %s", payload.ExitCode, msg)
		}
		return nil
	}
	logf("info", "%s", string(out))
	return nil
}

// execApproval pauses the run; the runner interprets this sentinel before
// calling the executor, so this body is only a defensive fallback.
func (s *Service) execApproval(ctx context.Context, run *Run, st *StageRun, logf func(level, format string, args ...any)) error {
	return errApprovalRequired
}

func readComposeForPipeline(workdir, composePath string) (string, error) {
	if composePath != "" {
		if strings.Contains(composePath, "..") {
			return "", fmt.Errorf("invalid compose path")
		}
		raw, err := os.ReadFile(filepath.Join(workdir, composePath))
		if err != nil {
			return "", fmt.Errorf("read compose file: %w", err)
		}
		return string(raw), nil
	}
	for _, name := range []string{"compose.yml", "compose.yaml", "docker-compose.yml", "docker-compose.yaml"} {
		if raw, err := os.ReadFile(filepath.Join(workdir, name)); err == nil {
			return string(raw), nil
		}
	}
	return "", fmt.Errorf("no compose file found in %s", workdir)
}