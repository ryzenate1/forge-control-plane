package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/services/build"
	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/deployment"
)

const (
	defaultMaxConcurrency = 2
	pollInterval          = 900 * time.Millisecond
)

// Options wires the pipeline service onto existing phase infrastructure. The
// service deliberately depends only on the published service types of the
// build/compose/deployment packages — no imports from other phase packages.
type Options struct {
	Store          *Store
	SharedStore    *store.Store
	Daemon         *daemon.Client
	BuildService   *build.Service
	ComposeService *compose.Service
	DeployService  *deployment.Service
	Logger         *slog.Logger
	DataDir        string
	// MaxConcurrency bounds parallel run execution. Defaults to 2; the
	// PIPELINE_MAX_CONCURRENCY env var wins when set.
	MaxConcurrency int
}

// Service owns pipeline persistence, the run queue worker pool and the
// artifact handler. Start the worker with Start(ctx); Stop waits for it.
type Service struct {
	store       *Store
	sharedStore *store.Store
	daemonCli   *daemon.Client
	buildSvc    *build.Service
	composeSvc  *compose.Service
	deploySvc   *deployment.Service
	logger      *slog.Logger
	artifacts   *ArtifactHandler
	concurrency int

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool

	// resume wakes the queue loop when a paused run is approved or when a
	// cancellation is requested, so actions resume without a poll delay.
	resume chan string
}

func New(opts Options) (*Service, error) {
	if opts.Store == nil {
		return nil, errors.New("pipeline store is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	concurrency := opts.MaxConcurrency
	if concurrency <= 0 {
		concurrency = defaultMaxConcurrency
	}
	if env := os.Getenv("PIPELINE_MAX_CONCURRENCY"); env != "" {
		if n, err := strconv.Atoi(env); err == nil && n > 0 {
			concurrency = n
		}
	}
	return &Service{
		store:       opts.Store,
		daemonCli:   opts.Daemon,
		buildSvc:    opts.BuildService,
		composeSvc:  opts.ComposeService,
		deploySvc:   opts.DeployService,
		logger:      opts.Logger,
		artifacts:   NewArtifactHandler(opts.DataDir),
		concurrency: concurrency,
		resume:      make(chan string, 64),
	}, nil
}

// Start launches the queue worker loop.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer guardRuntime("pipeline queue loop", s.logger)
		s.queueLoop(ctx)
	}()
}

func (s *Service) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func guardRuntime(what string, logger *slog.Logger) {
	if r := recover(); r != nil {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		logger.Error(what+" panicked",
			slog.String("panic", fmt.Sprintf("%v", r)),
			slog.String("stack", string(buf[:n])),
		)
	}
}

func (s *Service) queueLoop(ctx context.Context) {
	sem := make(chan struct{}, s.concurrency)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case runID := <-s.resume:
			s.runWithSemaphore(ctx, sem, runID)
		case <-ticker.C:
			claimed, err := s.store.ClaimQueuedRuns(ctx, s.concurrency)
			if err != nil {
				s.logger.Error("claim queued pipeline runs", slog.String("error", err.Error()))
				continue
			}
			for _, run := range claimed {
				s.runWithSemaphore(ctx, sem, run.ID)
			}
		}
	}
}

func (s *Service) runWithSemaphore(ctx context.Context, sem chan struct{}, runID string) {
	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			defer guardRuntime("pipeline run "+runID, s.logger)
			if err := s.ExecuteRun(context.WithoutCancel(ctx), runID); err != nil {
				s.logger.Error("pipeline run failed",
					slog.String("run_id", runID),
					slog.String("error", err.Error()),
				)
			}
		}()
	case <-ctx.Done():
	}
}

// ---- definition CRUD ----

func (s *Service) CreateDefinition(ctx context.Context, def *Definition) (*Definition, error) {
	if def.Name == "" {
		return nil, errors.New("pipeline name is required")
	}
	if def.Trigger.Type == "" {
		def.Trigger.Type = "manual"
	}
	if err := s.store.CreateDefinition(ctx, def); err != nil {
		return nil, err
	}
	return s.store.GetDefinition(ctx, def.ID)
}

func (s *Service) GetDefinition(ctx context.Context, id string) (*Definition, error) {
	return s.store.GetDefinition(ctx, id)
}

func (s *Service) ListDefinitions(ctx context.Context) ([]*Definition, error) {
	return s.store.ListDefinitions(ctx)
}

func (s *Service) UpdateDefinition(ctx context.Context, id string, def *Definition) (*Definition, error) {
	if def.Name == "" {
		return nil, errors.New("pipeline name is required")
	}
	if def.Trigger.Type == "" {
		def.Trigger.Type = "manual"
	}
	if err := s.store.UpdateDefinition(ctx, id, def); err != nil {
		return nil, err
	}
	return s.store.GetDefinition(ctx, id)
}

func (s *Service) DeleteDefinition(ctx context.Context, id string) error {
	return s.store.DeleteDefinition(ctx, id)
}

// TriggerRun freezes the definition's stages into a new queued run.
func (s *Service) TriggerRun(ctx context.Context, pipelineID, trigger, actor string) (*Run, error) {
	def, err := s.store.GetDefinition(ctx, pipelineID)
	if err != nil {
		return nil, err
	}
	if trigger == "" {
		trigger = def.Trigger.Type
	}
	if trigger == "" {
		trigger = "manual"
	}
	stages := s.snapshotStages(def)
	run := &Run{
		PipelineID:  def.ID,
		Trigger:     trigger,
		Status:      StatusQueued,
		RequestedBy: actor,
	}
	if err := s.store.CreateRun(ctx, run, stages); err != nil {
		return nil, err
	}
	s.append(run.ID, "", "info", fmt.Sprintf("pipeline %q queued (trigger: %s)", def.Name, trigger))
	return s.store.GetRun(ctx, run.ID)
}

// RetryRun creates a follow-up run for a finished run. The new run carries
// retryOf/retryCount so the failure lineage surfaces in the run list.
func (s *Service) RetryRun(ctx context.Context, runID, actor string) (*Run, error) {
	orig, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if orig.Status != StatusFailed && orig.Status != StatusCancelled {
		return nil, errors.New("only failed or cancelled runs can be retried")
	}
	def, err := s.store.GetDefinition(ctx, orig.PipelineID)
	if err != nil {
		return nil, err
	}
	retryOf := orig.ID
	run := &Run{
		PipelineID:  def.ID,
		Trigger:     "retry",
		Status:      StatusQueued,
		RetryOf:     &retryOf,
		RetryCount:  orig.RetryCount + 1,
		RequestedBy: actor,
	}
	if err := s.store.CreateRun(ctx, run, s.snapshotStages(def)); err != nil {
		return nil, err
	}
	s.append(run.ID, "", "info", fmt.Sprintf("retry of run %s", runID))
	return s.store.GetRun(ctx, run.ID)
}

func (s *Service) snapshotStages(def *Definition) []StageRun {
	stages := make([]StageRun, len(def.Stages))
	for i, st := range def.Stages {
		timeoutSec := st.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 600
		}
		retry := st.Retry
		if retry.BackoffMs == 0 {
			retry.BackoffMs = 1000
		}
		stages[i] = StageRun{
			Position:          i,
			Name:              st.Name,
			Action:            string(st.Action),
			Config:            cloneConfig(st.Config),
			Status:            StageStatusQueued,
			ContinueOnFailure: st.ContinueOnFailure,
			TimeoutSec:        timeoutSec,
			RetryPolicy:       retry,
		}
	}
	return stages
}

// ---- runs ----

func (s *Service) GetRun(ctx context.Context, runID string) (*Run, error) {
	return s.store.GetRun(ctx, runID)
}

func (s *Service) ListRuns(ctx context.Context, pipelineID, status string, limit, offset int) ([]*Run, error) {
	return s.store.ListRuns(ctx, pipelineID, status, limit, offset)
}

// CancelRun handles both queued and running runs; in-flight stage execution
// observes the cancel_requested flag between attempts.
func (s *Service) CancelRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	switch run.Status {
	case StatusQueued:
		if err := s.store.SetRunFinished(ctx, runID, StatusCancelled, "cancelled before start"); err != nil {
			return err
		}
		s.append(runID, "", "warn", "run cancelled while queued")
		return nil
	case StatusRunning, StatusAwaitApproval:
		marked, err := s.store.SetRunCancelRequested(ctx, runID)
		if err != nil {
			return err
		}
		if !marked {
			return errors.New("run is not in a cancellable state")
		}
		s.append(runID, "", "warn", "cancellation requested")
		select {
		case s.resume <- runID:
		default:
		}
		return nil
	}
	return errors.New("run already finished")
}

func (s *Service) ListLogs(ctx context.Context, runID string, after int64) ([]LogEntry, error) {
	return s.store.ListLogsAfter(ctx, runID, after, 500)
}

// ---- artifacts ----

func (s *Service) ListArtifacts(ctx context.Context, runID string) ([]Artifact, error) {
	return s.store.ListArtifacts(ctx, runID)
}

func (s *Service) GetArtifact(ctx context.Context, artifactID string) (*Artifact, error) {
	return s.store.GetArtifact(ctx, artifactID)
}

// UploadArtifact persists a payload with the local file handler and records
// metadata. Payloads are capped at 64 MiB by the HTTP layer too.
func (s *Service) UploadArtifact(ctx context.Context, runID, stageID, name, contentType string, payload []byte, actor string) (*Artifact, error) {
	if _, err := s.store.GetRun(ctx, runID); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("artifact name is required")
	}
	if len(payload) > 64*1024*1024 {
		return nil, errors.New("artifact exceeds 64 MiB limit")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	rel, err := s.artifacts.Save(runID, stageID, name, payload)
	if err != nil {
		return nil, err
	}
	art := &Artifact{
		RunID:        runID,
		StageID:      stageID,
		Name:         name,
		RelativePath: rel,
		SizeBytes:    int64(len(payload)),
		ContentType:  contentType,
		CreatedBy:    actor,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		return nil, err
	}
	s.append(runID, stageID, "info", fmt.Sprintf("artifact %s uploaded (%d bytes)", name, len(payload)))
	return art, nil
}

func (s *Service) ReadArtifact(ctx context.Context, artifactID string) (*Artifact, []byte, error) {
	art, err := s.store.GetArtifact(ctx, artifactID)
	if err != nil {
		return nil, nil, err
	}
	payload, err := s.artifacts.Read(art.RelativePath)
	if err != nil {
		return nil, nil, err
	}
	return art, payload, nil
}

func (s *Service) DeleteArtifact(ctx context.Context, artifactID string) error {
	art, err := s.store.DeleteArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if err := s.artifacts.Delete(art.RelativePath); err != nil {
		return err
	}
	s.append(art.RunID, art.StageID, "warn", fmt.Sprintf("artifact %s deleted", art.Name))
	return nil
}

// ---- approval gates ----

// ApproveStage resumes a run paused at an approval stage; the queue loop
// re-enters the run through the resume channel.
func (s *Service) ApproveStage(ctx context.Context, runID, stageID, actor string) error {
	stage, err := s.store.GetStageRun(ctx, stageID)
	if err != nil {
		return err
	}
	if stage.RunID != runID {
		return errors.New("stage does not belong to run")
	}
	if stage.Status != StageStatusAwaiting {
		return errors.New("stage is not awaiting approval")
	}
	if err := s.store.ResetStageRun(ctx, runID, stageID); err != nil {
		return err
	}
	if err := s.store.MarkRunProgress(ctx, runID, StatusRunning, stage.Name, 0, ""); err != nil {
		return err
	}
	s.append(runID, stageID, "info", fmt.Sprintf("stage %q approved by %s", stage.Name, actor))
	select {
	case s.resume <- runID:
	default:
	}
	return nil
}

// RejectStage fails the run at an approval gate.
func (s *Service) RejectStage(ctx context.Context, runID, stageID, actor string) error {
	stage, err := s.store.GetStageRun(ctx, stageID)
	if err != nil {
		return err
	}
	if stage.RunID != runID {
		return errors.New("stage does not belong to run")
	}
	if stage.Status != StageStatusAwaiting {
		return errors.New("stage is not awaiting approval")
	}
	if err := s.store.SetStageStatus(ctx, stageID, StageStatusFailed, "rejected by "+actor); err != nil {
		return err
	}
	s.append(runID, stageID, "error", fmt.Sprintf("stage %q rejected by %s", stage.Name, actor))
	return s.store.SetRunFinished(ctx, runID, StatusFailed, "approval rejected")
}

// ---- execution engine ----

// ExecuteRun drives a claimed (status=running) run through its ordered
// stages. Re-entrant by design: approved runs re-enter at the resumed stage.
func (s *Service) ExecuteRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.CancelRequested {
		return s.store.SetRunFinished(ctx, runID, StatusCancelled, "cancelled before resume")
	}
	for i := range run.Stages {
		st := &run.Stages[i]
		if st.Status == StageStatusSucceeded || st.Status == StageStatusSkipped || st.Status == StageStatusCancelled {
			continue
		}
		err := s.executeStage(ctx, run, st)
		if errors.Is(err, errApprovalRequired) {
			pct := progressPct(i, len(run.Stages))
			return s.store.MarkRunProgress(ctx, runID, StatusAwaitApproval, st.Name, pct, "")
		}
		if err != nil {
			s.append(runID, st.ID, "error", fmt.Sprintf("stage %q failed: %v", st.Name, err))
			if errors.Is(err, errStageCancelled) {
				return s.store.SetRunFinished(ctx, runID, StatusCancelled, "cancelled during execution")
			}
			if st.Status == StageStatusFailed && st.ContinueOnFailure {
				s.append(runID, st.ID, "warn", "continuing past failed stage (continueOnFailure)")
				if err := s.store.MarkRunProgress(ctx, runID, StatusRunning, st.Name, progressPct(i, len(run.Stages)), ""); err != nil {
					return err
				}
				continue
			}
			return s.store.SetRunFinished(ctx, runID, StatusFailed, err.Error())
		}
		if st.Status == StageStatusSucceeded {
			s.append(runID, st.ID, "info", fmt.Sprintf("stage %q succeeded", st.Name))
		}
		if run.CancelRequested {
			return s.store.SetRunFinished(ctx, runID, StatusCancelled, "cancelled between stages")
		}
		if err := s.store.MarkRunProgress(ctx, runID, StatusRunning, st.Name, progressPct(i+1, len(run.Stages)), ""); err != nil {
			return err
		}
	}
	s.append(runID, "", "info", "run completed")
	return s.store.SetRunFinished(ctx, runID, StatusCompleted, "")
}

func progressPct(atStage, total int) int {
	if total <= 0 {
		return 100
	}
	pct := (atStage * 100) / total
	if pct > 100 {
		pct = 100
	}
	return pct
}

var errStageCancelled = errors.New("stage cancelled")

func (s *Service) executeStage(ctx context.Context, run *Run, st *StageRun) error {
	action := Action(st.Action)
	executor, ok := s.executorFor(action)
	if !ok {
		s.append(run.ID, st.ID, "error", fmt.Sprintf("unsupported action %q", action))
		_ = s.store.SetStageStatus(ctx, st.ID, StageStatusFailed, "unsupported action "+string(action))
		return fmt.Errorf("unsupported action %q", action)
	}
	if action == ActionApproval {
		s.append(run.ID, st.ID, "warn", fmt.Sprintf("stage %q requires approval", st.Name))
		if err := s.store.SetStageStatus(ctx, st.ID, StageStatusAwaiting, ""); err != nil {
			return err
		}
		return errApprovalRequired
	}

	logf := func(level, format string, args ...any) {
		message := fmt.Sprintf("[%s] %s", st.Name, fmt.Sprintf(format, args...))
		switch level {
		case "warn":
			s.append(run.ID, st.ID, "warn", message)
		case "error":
			s.append(run.ID, st.ID, "error", message)
		default:
			s.append(run.ID, st.ID, "info", message)
		}
	}

	if err := s.store.StartStage(ctx, st.ID); err != nil {
		return err
	}
	timeoutSec := st.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	maxRetries := st.RetryPolicy.MaxRetries
	backoff := st.RetryPolicy.BackoffMs
	if backoff <= 0 {
		backoff = 1000
	}
	attempt := 0
	for {
		attempt++
		stageCtx, cancelStage := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		runErr := executor(stageCtx, run, st, logf)
		cancelStage()
		if runErr == nil {
			if err := s.store.SetStageStatus(ctx, st.ID, StageStatusSucceeded, ""); err != nil {
				return err
			}
			return nil
		}
		if errors.Is(runErr, context.Canceled) || run.CancelRequested {
			s.append(run.ID, st.ID, "warn", "stage interrupted by cancellation")
			_ = s.store.SetStageStatus(ctx, st.ID, StageStatusCancelled, "cancelled")
			return errStageCancelled
		}
		if errors.Is(runErr, context.DeadlineExceeded) {
			s.append(run.ID, st.ID, "error", fmt.Sprintf("stage timed out after %d seconds", timeoutSec))
			_ = s.store.SetStageStatus(ctx, st.ID, StageStatusFailed, "timeout")
			return runErr
		}
		if attempt <= maxRetries {
			s.append(run.ID, st.ID, "warn", fmt.Sprintf("stage failed (attempt %d/%d): %v — retrying in %dms", attempt, maxRetries+1, runErr, backoff))
			select {
			case <-time.After(time.Duration(backoff) * time.Millisecond):
			case <-ctx.Done():
				_ = s.store.SetStageStatus(ctx, st.ID, StageStatusCancelled, "cancelled")
				return errStageCancelled
			}
			_ = s.store.StartStage(ctx, st.ID)
			backoff *= 2
			if maxBackoff := st.RetryPolicy.MaxSleepMs; maxBackoff > 0 && backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		_ = s.store.SetStageStatus(ctx, st.ID, StageStatusFailed, runErr.Error())
		return runErr
	}
}

func (s *Service) append(runID, stageID, level, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.store.AppendLog(ctx, runID, stageID, level, message); err != nil {
		s.logger.Error("append pipeline log", slog.String("error", err.Error()))
	}
}
func cloneConfig(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
