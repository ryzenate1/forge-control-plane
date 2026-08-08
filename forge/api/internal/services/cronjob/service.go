package cronjob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type Service struct {
	store   *store.Store
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[string]cron.EntryID
	logger  *slog.Logger
}

func New(s *store.Store, logger *slog.Logger) (*Service, error) {
	if s == nil {
		return nil, errors.New("store required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:   s,
		cron:    cron.New(),
		entries: make(map[string]cron.EntryID),
		logger:  logger,
	}, nil
}

func (s *Service) Start(ctx context.Context) error {
	jobs, err := s.store.ListEnabledCronJobs(ctx)
	if err != nil {
		return fmt.Errorf("load cron jobs: %w", err)
	}
	for _, job := range jobs {
		if err := s.scheduleJob(ctx, job); err != nil {
			s.logger.Error("failed to schedule cron job", "id", job.ID, "name", job.Name, "error", err)
		}
	}
	s.cron.Start()
	return nil
}

func (s *Service) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Service) scheduleJob(ctx context.Context, job store.CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.entries[job.ID]; ok {
		s.cron.Remove(existingID)
	}

	jobID := job.ID
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		s.executeJob(context.Background(), jobID)
	})
	if err != nil {
		return err
	}
	s.entries[job.ID] = entryID
	return nil
}

func (s *Service) removeJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[jobID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, jobID)
	}
}

func (s *Service) RescheduleJob(ctx context.Context, job store.CronJob) error {
	if !job.Enabled {
		s.removeJob(job.ID)
		return nil
	}
	return s.scheduleJob(ctx, job)
}

func (s *Service) executeJob(ctx context.Context, jobID string) {
	job, err := s.store.GetCronJob(ctx, jobID)
	if err != nil {
		s.logger.Error("cron job not found for execution", "id", jobID, "error", err)
		return
	}

	execution, err := s.store.CreateCronJobExecution(ctx, jobID)
	if err != nil {
		s.logger.Error("failed to create execution record", "id", jobID, "error", err)
		return
	}

	start := time.Now()
	var exitCode int
	var output, errStr string

	switch job.Type {
	case "shell":
		exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
	default:
		exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
	}

	durationMs := int(time.Since(start).Milliseconds())

	status := "success"
	if exitCode != 0 {
		status = "failed"
	}

	if err := s.store.CompleteCronJobExecution(ctx, execution.ID, status, exitCode, output, errStr, durationMs); err != nil {
		s.logger.Error("failed to complete execution record", "id", execution.ID, "error", err)
	}

	if status == "failed" && job.RetryCount > 0 {
		for i := 0; i < job.RetryCount; i++ {
			s.logger.Info("retrying cron job", "id", jobID, "attempt", i+1)
			time.Sleep(time.Duration(5*(i+1)) * time.Second)

			retryExec, err := s.store.CreateCronJobExecution(ctx, jobID)
			if err != nil {
				continue
			}
			retryStart := time.Now()
			exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
			retryDuration := int(time.Since(retryStart).Milliseconds())
			retryStatus := "success"
			if exitCode != 0 {
				retryStatus = "failed"
			}
			_ = s.store.CompleteCronJobExecution(ctx, retryExec.ID, retryStatus, exitCode, output, errStr, retryDuration)
			if exitCode == 0 {
				break
			}
		}
	}
}

// defaultMaxCronTimeoutSeconds bounds how long a single cron job execution is
// allowed to run when the job's configured timeout is missing or invalid.
// This is a defense-in-depth safeguard so a misconfigured job can't run (or
// hang) indefinitely.
const defaultMaxCronTimeoutSeconds = 3600 // 1 hour

// runShellCommand executes an administrator-configured cron job command via
// "sh -c". This is intentionally unsanitized: cron jobs are free-form shell
// commands by design (pipelines, redirection, env expansion, etc.), so
// rejecting "dangerous" shell metacharacters would break legitimate use cases
// without meaningfully improving security.
//
// Security posture (defense-in-depth, since arbitrary shell execution here is
// intentional functionality, not a bug):
//   - Authorization: creating/editing cron jobs MUST be restricted to
//     admin/owner roles at the HTTP handler layer. This function trusts that
//     anything it is asked to execute has already been authored by a
//     trusted, privileged operator - it does not re-validate the command
//     text itself.
//   - Bounded execution: every invocation runs under a hard timeout so a
//     runaway or hung command cannot block the scheduler indefinitely.
//   - Minimal environment: the child process inherits a scrubbed environment
//     (only PATH/HOME) rather than the full environment of the API process,
//     reducing the blast radius if a command is compromised or malicious
//     (e.g. it cannot read unrelated secrets present in the parent
//     process's environment).
func (s *Service) runShellCommand(command string, timeoutSeconds int) (int, string, string) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultMaxCronTimeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/tmp",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, "", "command timed out"
		}
		exitCode = 1
	}

	return exitCode, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String())
}

func (s *Service) TriggerNow(ctx context.Context, jobID string) (store.CronJobExecution, error) {
	if s.store == nil {
		return store.CronJobExecution{}, fmt.Errorf("store not initialized")
	}

	execution, err := s.store.CreateCronJobExecution(ctx, jobID)
	if err != nil {
		return store.CronJobExecution{}, err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				s.logger.Error("cron job trigger panic recovered", "job_id", jobID, "panic", r, "stack", string(buf[:n]))
			}
		}()
		s.executeJob(context.Background(), jobID)
	}()

	return execution, nil
}

func (s *Service) NextRun(job store.CronJob) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID, ok := s.entries[job.ID]
	if !ok {
		return nil
	}

	entry := s.cron.Entry(entryID)
	if entry.Next.IsZero() {
		return nil
	}
	return &entry.Next
}

func (s *Service) GenerateExecutionID() string {
	return uuid.NewString()
}
