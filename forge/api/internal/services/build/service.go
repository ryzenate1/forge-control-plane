// Package build orchestrates container image builds. The panel owns the
// build record lifecycle while the image is built either locally or on a
// Beacon node, so a dispatched build is always persisted before it starts and
// Start resumes monitoring after an API restart.
package build

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type builder interface {
	Detect(sourceDir string) bool
	Build(ctx context.Context, opts BuildOptions) ([]string, error)
}

type Service struct {
	store     *store.Store
	daemonCli *daemon.Client
	logger    *slog.Logger

	builders map[BuilderType]builder

	mu         sync.RWMutex
	logStreams map[string]chan BuildLogEntry
}

func NewService(st *store.Store, daemonCli *daemon.Client, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:      st,
		daemonCli:  daemonCli,
		logger:     logger,
		logStreams: make(map[string]chan BuildLogEntry),
		builders: map[BuilderType]builder{
			BuilderDockerfile: dockerfileBuilder{},
			BuilderNixpacks:   nixpacksBuilder{},
		},
	}
}

// Start reaps stale records and re-attaches to builds that were in flight
// when the API stopped.
func (s *Service) Start(ctx context.Context) error {
	if err := s.store.ReapAbandonedBuilds(ctx); err != nil {
		return fmt.Errorf("reap abandoned builds: %w", err)
	}
	if err := s.recoverRunningBuilds(ctx); err != nil {
		return fmt.Errorf("recover running builds: %w", err)
	}
	go s.periodicReaper(ctx)
	return nil
}

func (s *Service) periodicReaper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.store.ReapAbandonedBuilds(ctx); err != nil {
				s.logger.Warn("build reaper failed", slog.String("error", err.Error()))
			}
		}
	}
}

func (s *Service) recoverRunningBuilds(ctx context.Context) error {
	records, err := s.store.ListNonTerminalBuilds(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.BeaconBuildID != "" && record.NodeID != "" {
			go s.monitorRemoteBuild(record.ID, record.NodeID, record.BeaconBuildID)
			continue
		}
		if time.Since(record.StartedAt) > 5*time.Minute {
			s.finishBuild(ctx, record, BuildAbandoned, -1, "build was interrupted by an API restart")
		}
	}
	return nil
}

// Detect resolves the builder for a source tree, preferring an explicit
// Dockerfile over Nixpacks autodetection.
func (s *Service) Detect(sourceDir string) (BuilderType, error) {
	if strings.TrimSpace(sourceDir) == "" {
		return "", errors.New("sourceDir is required")
	}
	if s.builders[BuilderDockerfile].Detect(sourceDir) {
		return BuilderDockerfile, nil
	}
	if s.builders[BuilderNixpacks].Detect(sourceDir) {
		return BuilderNixpacks, nil
	}
	return "", errors.New("no supported builder detected for this source")
}

func (s *Service) StartBuild(ctx context.Context, sourceID string, builderType BuilderType, opts BuildOptions, logCh chan<- BuildLogEntry) (*store.BuildRecord, error) {
	if builderType == "" {
		detected, err := s.Detect(opts.SourceDir)
		if err != nil {
			return nil, err
		}
		builderType = detected
	}
	if _, ok := s.builders[builderType]; !ok {
		return nil, fmt.Errorf("unknown builder type %q", builderType)
	}
	if opts.BuildTimeout <= 0 {
		opts.BuildTimeout = 1800
	}
	if opts.Platform == "" {
		opts.Platform = "linux/amd64"
	}

	idempotencyKey := opts.BuildIdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = buildIdempotencyKey(sourceID, opts)
	}
	if existing, err := s.store.GetActiveBuildByIdempotencyKey(ctx, idempotencyKey); err == nil && existing != nil {
		return existing, nil
	}

	record := &store.BuildRecord{
		ID:             uuid.NewString(),
		SourceID:       sourceID,
		BuilderType:    string(builderType),
		Status:         string(BuildRunning),
		BuildStage:     string(BuildStageQueued),
		StartedAt:      time.Now().UTC(),
		NodeID:         opts.NodeID,
		Registry:       opts.Registry,
		CacheFrom:      opts.CacheFrom,
		CacheTo:        opts.CacheTo,
		Platform:       opts.Platform,
		CommitSHA:      opts.CommitSHA,
		CommitRef:      opts.CommitRef,
		BuildTimeout:   opts.BuildTimeout,
		IdempotencyKey: idempotencyKey,
	}
	if err := s.store.CreateBuild(ctx, record); err != nil {
		return nil, fmt.Errorf("create build record: %w", err)
	}

	stream := make(chan BuildLogEntry, 256)
	s.mu.Lock()
	s.logStreams[record.ID] = stream
	s.mu.Unlock()

	if logCh != nil {
		go func() {
			for entry := range stream {
				select {
				case logCh <- entry:
				default:
				}
			}
		}()
	} else {
		go func() {
			for range stream {
			}
		}()
	}

	if opts.NodeID != "" {
		go s.runRemoteBuild(context.WithoutCancel(ctx), record, builderType, opts)
	} else {
		go s.runLocalBuild(context.WithoutCancel(ctx), record, builderType, opts)
	}
	return record, nil
}

func (s *Service) GetBuild(ctx context.Context, id string) (*store.BuildRecord, error) {
	return s.store.GetBuild(ctx, id)
}

func (s *Service) ListBuilds(ctx context.Context, sourceID string) ([]*store.BuildRecord, error) {
	return s.store.ListBuilds(ctx, sourceID)
}

// StreamLogs follows a live build, or replays the persisted log when the
// build has already finished.
func (s *Service) StreamLogs(ctx context.Context, buildID string) (<-chan BuildLogEntry, error) {
	s.mu.RLock()
	live, ok := s.logStreams[buildID]
	s.mu.RUnlock()

	out := make(chan BuildLogEntry, 256)
	if ok {
		go func() {
			defer close(out)
			for {
				select {
				case entry, open := <-live:
					if !open {
						return
					}
					select {
					case out <- entry:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		return out, nil
	}

	record, err := s.store.GetBuild(ctx, buildID)
	if err != nil {
		return nil, err
	}
	go func() {
		defer close(out)
		for _, line := range strings.Split(record.BuildLog, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			select {
			case out <- BuildLogEntry{BuildID: buildID, Line: line, Timestamp: record.StartedAt}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *Service) CancelBuild(ctx context.Context, buildID string) error {
	record, err := s.store.GetBuild(ctx, buildID)
	if err != nil {
		return err
	}
	if IsTerminal(BuildStatus(record.Status)) {
		return nil
	}
	if record.BeaconBuildID != "" && record.NodeID != "" {
		baseURL, token, err := s.nodeTarget(ctx, record.NodeID)
		if err != nil {
			return err
		}
		if err := s.daemonCli.CancelBuild(ctx, baseURL, token, record.BeaconBuildID); err != nil {
			return err
		}
	} else if record.PID != nil && *record.PID > 0 {
		killPID(*record.PID)
	}
	s.finishBuild(ctx, record, BuildCanceled, -1, "build canceled by operator")
	return nil
}

func (s *Service) nodeTarget(ctx context.Context, nodeID string) (string, string, error) {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return "", "", fmt.Errorf("resolve node: %w", err)
	}
	token, err := s.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return "", "", fmt.Errorf("resolve node credential: %w", err)
	}
	return node.BaseURL, token, nil
}

func (s *Service) runRemoteBuild(ctx context.Context, record *store.BuildRecord, builderType BuilderType, opts BuildOptions) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.BuildTimeout)*time.Second)
	defer cancel()

	baseURL, token, err := s.nodeTarget(ctx, opts.NodeID)
	if err != nil {
		s.finishBuild(ctx, record, BuildFailed, -1, err.Error())
		return
	}
	if opts.RegistryAuth != nil {
		if err := s.daemonCli.LoginRegistry(ctx, baseURL, token, *opts.RegistryAuth); err != nil {
			s.logger.Warn("registry login failed", slog.String("build", record.ID), slog.String("error", err.Error()))
		}
	}

	record.BuildStage = string(BuildStageBuilding)
	_ = s.store.UpdateBuild(ctx, record)

	workspaceID := strings.TrimPrefix(opts.SourceDir, "server:")
	var resp *daemon.BuildStartResponse
	switch builderType {
	case BuilderDockerfile:
		resp, err = s.daemonCli.DockerfileBuild(ctx, baseURL, token, daemon.DockerfileBuildRequest{
			WorkspaceID:    workspaceID,
			SourceDir:      opts.SourceDir,
			Dockerfile:     opts.Dockerfile,
			ImageName:      opts.ImageName,
			BuildArgs:      opts.BuildArgs,
			Labels:         opts.Labels,
			Tags:           opts.Tags,
			NoCache:        opts.NoCache,
			CacheFrom:      opts.CacheFrom,
			CacheTo:        opts.CacheTo,
			Platform:       opts.Platform,
			RegistryAuth:   opts.RegistryAuth,
			IdempotencyKey: record.IdempotencyKey,
		})
	case BuilderNixpacks:
		resp, err = s.daemonCli.NixpacksBuild(ctx, baseURL, token, daemon.NixpacksBuildRequest{
			WorkspaceID:    workspaceID,
			SourceDir:      opts.SourceDir,
			ImageName:      opts.ImageName,
			BuildArgs:      opts.BuildArgs,
			Tags:           opts.Tags,
			NoCache:        opts.NoCache,
			Platform:       opts.Platform,
			RegistryAuth:   opts.RegistryAuth,
			IdempotencyKey: record.IdempotencyKey,
		})
	}
	if err != nil {
		s.finishBuild(ctx, record, BuildFailed, -1, err.Error())
		return
	}

	record.BeaconBuildID = resp.ID
	record.ImageRef = firstNonEmpty(opts.ImageName, resp.ImageName)
	_ = s.store.UpdateBuild(ctx, record)

	s.monitorRemoteBuild(record.ID, opts.NodeID, resp.ID)
}

func (s *Service) monitorRemoteBuild(buildID, nodeID, beaconBuildID string) {
	ctx := context.Background()
	record, err := s.store.GetBuild(ctx, buildID)
	if err != nil {
		return
	}
	baseURL, token, err := s.nodeTarget(ctx, nodeID)
	if err != nil {
		s.finishBuild(ctx, record, BuildFailed, -1, err.Error())
		return
	}

	if logs, err := s.daemonCli.BuildLogs(ctx, baseURL, token, beaconBuildID, true); err == nil {
		for _, line := range logs {
			s.publishLog(ctx, record, line.Line)
		}
	}

	timeout := time.Duration(record.BuildTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	deadline := record.StartedAt.Add(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		status, err := s.daemonCli.GetBuildStatus(ctx, baseURL, token, beaconBuildID)
		if err == nil {
			switch BuildStatus(status.Status) {
			case BuildSucceeded:
				s.finishBuild(ctx, record, BuildSucceeded, 0, "")
				return
			case BuildFailed:
				s.finishBuild(ctx, record, BuildFailed, status.ExitCode, "remote build failed")
				return
			case BuildCanceled:
				s.finishBuild(ctx, record, BuildCanceled, status.ExitCode, "remote build canceled")
				return
			}
		}
		<-ticker.C
	}
	s.finishBuild(ctx, record, BuildAbandoned, -1, "remote build did not finish before its timeout")
}

func (s *Service) runLocalBuild(ctx context.Context, record *store.BuildRecord, builderType BuilderType, opts BuildOptions) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.BuildTimeout)*time.Second)
	defer cancel()

	record.BuildStage = string(BuildStageBuilding)
	_ = s.store.UpdateBuild(ctx, record)

	lines, err := s.builders[builderType].Build(ctx, opts)
	for _, line := range lines {
		s.publishLog(ctx, record, line)
	}
	if err != nil {
		s.finishBuild(ctx, record, BuildFailed, -1, err.Error())
		return
	}
	if opts.ImageName != "" {
		record.ImageRef = opts.ImageName
	}
	s.finishBuild(ctx, record, BuildSucceeded, 0, "")
}

func (s *Service) publishLog(ctx context.Context, record *store.BuildRecord, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if record.BuildLog != "" {
		record.BuildLog += "\n"
	}
	record.BuildLog += line
	_ = s.store.UpdateBuild(ctx, record)

	s.mu.RLock()
	stream, ok := s.logStreams[record.ID]
	s.mu.RUnlock()
	if ok {
		select {
		case stream <- BuildLogEntry{BuildID: record.ID, Line: line, Timestamp: time.Now().UTC()}:
		default:
		}
	}
}

func (s *Service) finishBuild(ctx context.Context, record *store.BuildRecord, status BuildStatus, exitCode int, message string) {
	record.Status = string(status)
	record.BuildStage = string(BuildStageBuilt)
	record.ExitCode = &exitCode
	record.ErrorMessage = message
	now := time.Now().UTC()
	record.FinishedAt = &now
	if err := s.store.UpdateBuild(ctx, record); err != nil {
		s.logger.Warn("persist build result failed", slog.String("build", record.ID), slog.String("error", err.Error()))
	}

	s.mu.Lock()
	stream, ok := s.logStreams[record.ID]
	delete(s.logStreams, record.ID)
	s.mu.Unlock()
	if ok {
		close(stream)
	}
}

type dockerfileBuilder struct{}

func (dockerfileBuilder) Detect(sourceDir string) bool {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), "dockerfile") {
			return true
		}
	}
	return false
}

func (dockerfileBuilder) Build(ctx context.Context, opts BuildOptions) ([]string, error) {
	if err := validateBuildContext(opts.SourceDir); err != nil {
		return nil, err
	}
	dockerfile := opts.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(opts.SourceDir, "Dockerfile")
	}
	args := []string{"buildx", "build", "-f", dockerfile, "--load"}
	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	for _, arg := range opts.BuildArgs {
		args = append(args, "--build-arg", arg)
	}
	for _, label := range opts.Labels {
		args = append(args, "--label", label)
	}
	for _, tag := range opts.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageNameOrDefault(opts.ImageName))
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	args = append(args, opts.SourceDir)
	return runBuildCommand(ctx, "docker", args)
}

type nixpacksBuilder struct{}

func (nixpacksBuilder) Detect(sourceDir string) bool {
	if (dockerfileBuilder{}).Detect(sourceDir) {
		return false
	}
	indicators := []string{
		"package.json", "requirements.txt", "Pipfile", "pyproject.toml", "setup.py",
		"Gemfile", "pom.xml", "build.gradle", "build.gradle.kts", "go.mod",
		"Cargo.toml", "composer.json", "mix.exs",
	}
	for _, name := range indicators {
		if _, err := os.Stat(filepath.Join(sourceDir, name)); err == nil {
			return true
		}
	}
	return false
}

func (nixpacksBuilder) Build(ctx context.Context, opts BuildOptions) ([]string, error) {
	if err := validateBuildContext(opts.SourceDir); err != nil {
		return nil, err
	}
	args := []string{"build", opts.SourceDir, "--name", imageNameOrDefault(opts.ImageName)}
	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	for _, arg := range opts.BuildArgs {
		args = append(args, "--env", arg)
	}
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	if len(opts.NixpacksPlan) > 0 {
		planFile, err := writeNixpacksPlan(opts.NixpacksPlan)
		if err != nil {
			return nil, err
		}
		defer os.Remove(planFile)
		args = append(args, "--plan", planFile)
	}
	return runBuildCommand(ctx, "nixpacks", args)
}

func imageNameOrDefault(imageName string) string {
	if strings.TrimSpace(imageName) != "" {
		return imageName
	}
	return "forge/build:" + uuid.NewString()[:8]
}

// writeNixpacksPlan persists a build plan with owner-only permissions because
// plans can carry build-time environment values.
func writeNixpacksPlan(plan map[string]any) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "forge-nixpacks-plan-*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func validateBuildContext(sourceDir string) error {
	sourceDir = strings.TrimSpace(sourceDir)
	if sourceDir == "" {
		return errors.New("sourceDir is required")
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		return fmt.Errorf("source directory not found: %w", err)
	}
	if !info.IsDir() {
		return errors.New("sourceDir must be a directory")
	}
	return nil
}

// runBuildCommand executes a build binary from a fixed allowlist in its own
// process group, so a cancelled build can be killed together with children.
func runBuildCommand(ctx context.Context, name string, args []string) ([]string, error) {
	switch name {
	case "docker", "nixpacks":
	default:
		return nil, fmt.Errorf("unsupported build command %q", name)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	setSysProcAttr(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	lines := readCommandOutput(io.MultiReader(stdout, stderr))
	return lines, cmd.Wait()
}

func readCommandOutput(r io.Reader) []string {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func buildIdempotencyKey(sourceID string, opts BuildOptions) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		sourceID, opts.CommitSHA, opts.Dockerfile, opts.Platform, opts.Registry, opts.SourceDir,
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
