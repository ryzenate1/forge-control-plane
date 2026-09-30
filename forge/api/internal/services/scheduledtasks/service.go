package scheduledtasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

// Sentinel errors mapped to HTTP statuses by the handler layer.
var (
	// ErrNotFound is returned (wrapped) when a task or run does not exist.
	ErrNotFound = errors.New("scheduled task not found")
	// ErrValidation is returned (wrapped) for user-correctable input errors
	// such as an invalid cron expression or an empty command.
	ErrValidation = errors.New("invalid scheduled task")
	// ErrDispatch is returned (wrapped) when Beacon could not be reached or
	// rejected the command; the run record is still persisted as failed.
	ErrDispatch = errors.New("dispatch failed")
)

// maxCommandLen mirrors the daemon client's single-line command limit.
const maxCommandLen = 4096

// dispatchTimeout bounds one Beacon round-trip. The console write itself is
// instantaneous; the timeout only guards a wedged node.
const dispatchTimeout = 30 * time.Second

// CommandDispatcher executes a one-line command inside a server's container
// via Beacon. *daemon.Client satisfies this; tests can stub it.
type CommandDispatcher interface {
	SendCommandWithOutput(ctx context.Context, baseURL, nodeToken, serverID, command string) (string, error)
}

// Service owns scheduled-task CRUD, cron validation, manual runs and the
// background scheduler loop (see scheduler.go).
type Service struct {
	store      *store.Store
	dispatcher CommandDispatcher
	logger     *slog.Logger
	// parser accepts exactly the standard 5-field crontab grammar (the same
	// one Dokku crontabs and schedule_runner.go use), deliberately excluding
	// @every/@daily descriptors so stored expressions stay portable.
	parser cron.Parser
	// instanceID identifies this API replica for the scheduler leader lease.
	instanceID string
}

// New builds the service. store must not be nil; dispatcher may be nil, in
// which case task execution fails loudly instead of pretending to succeed.
func New(st *store.Store, dispatcher CommandDispatcher, logger *slog.Logger, instanceID string) (*Service, error) {
	if st == nil {
		return nil, errors.New("scheduledtasks: store required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if instanceID == "" {
		instanceID = fmt.Sprintf("api-%d", time.Now().UnixNano())
	}
	return &Service{
		store:      st,
		dispatcher: dispatcher,
		logger:     logger,
		parser:     cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		instanceID: instanceID,
	}, nil
}

// ValidateSchedule reports whether expr is a legal 5-field cron expression.
func (s *Service) ValidateSchedule(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("%w: schedule is required", ErrValidation)
	}
	if len(expr) > 64 {
		return fmt.Errorf("%w: schedule too long (max 64 characters)", ErrValidation)
	}
	if _, err := s.parser.Parse(expr); err != nil {
		return fmt.Errorf("%w: invalid cron expression %q: %v", ErrValidation, expr, err)
	}
	return nil
}

// nextRun computes the occurrence strictly after from.
func (s *Service) nextRun(schedule string, from time.Time) (time.Time, error) {
	sched, err := s.parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cron expression %q: %v", ErrValidation, schedule, err)
	}
	return sched.Next(from), nil
}

// CreateTask validates input, seeds next_run_at and persists the task.
func (s *Service) CreateTask(ctx context.Context, in CreateInput) (*ScheduledTask, error) {
	name := strings.TrimSpace(in.Name)
	command := strings.TrimSpace(in.Command)
	schedule := strings.TrimSpace(in.Schedule)
	serverID := strings.TrimSpace(in.ServerID)
	if serverID == "" {
		return nil, fmt.Errorf("%w: server id is required", ErrValidation)
	}
	if name == "" || len(name) > 128 {
		return nil, fmt.Errorf("%w: name is required (max 128 characters)", ErrValidation)
	}
	if err := validateCommand(command); err != nil {
		return nil, err
	}
	if err := s.ValidateSchedule(schedule); err != nil {
		return nil, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	var nextRun *time.Time
	if enabled {
		now := time.Now().UTC()
		next, err := s.nextRun(schedule, now)
		if err != nil {
			return nil, err
		}
		nextRun = &next
	}

	task, err := s.store.CreateScheduledTask(ctx, store.CreateScheduledTaskRequest{
		ServerID:  serverID,
		Name:      name,
		Command:   command,
		Schedule:  schedule,
		Enabled:   enabled,
		NextRunAt: nextRun,
		CreatedBy: in.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create scheduled task: %w", err)
	}
	return &task, nil
}

func validateCommand(command string) error {
	if command == "" {
		return fmt.Errorf("%w: command is required", ErrValidation)
	}
	if len(command) > maxCommandLen {
		return fmt.Errorf("%w: command too long (max %d characters)", ErrValidation, maxCommandLen)
	}
	// Beacon's command channel writes one line to the container console
	// stdin, so multi-line commands can never execute correctly. Reject
	// them up front instead of silently truncating.
	if strings.ContainsAny(command, "\n\r\x00") {
		return fmt.Errorf("%w: command must be a single line", ErrValidation)
	}
	return nil
}

// GetTask fetches a task by id.
func (s *Service) GetTask(ctx context.Context, taskID string) (*ScheduledTask, error) {
	task, err := s.store.GetScheduledTask(ctx, taskID)
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &task, nil
}

// GetTaskForServer fetches a task and enforces it belongs to serverID, so a
// guessed task id can never leak across the server-scoped route.
func (s *Service) GetTaskForServer(ctx context.Context, serverID, taskID string) (*ScheduledTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.ServerID != strings.TrimSpace(serverID) {
		return nil, fmt.Errorf("%w: task %q is not attached to server %q", ErrNotFound, taskID, serverID)
	}
	return task, nil
}

// ListTasks returns every task for a server, newest first.
func (s *Service) ListTasks(ctx context.Context, serverID string) ([]ScheduledTask, error) {
	tasks, err := s.store.ListScheduledTasksByServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("list scheduled tasks: %w", err)
	}
	return tasks, nil
}

// UpdateTask applies a patch; schedule/enabled changes recompute next_run_at.
func (s *Service) UpdateTask(ctx context.Context, serverID, taskID string, in UpdateInput) (*ScheduledTask, error) {
	task, err := s.GetTaskForServer(ctx, serverID, taskID)
	if err != nil {
		return nil, err
	}

	req := store.UpdateScheduledTaskRequest{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 128 {
			return nil, fmt.Errorf("%w: name is required (max 128 characters)", ErrValidation)
		}
		req.Name = &name
	}
	if in.Command != nil {
		command := strings.TrimSpace(*in.Command)
		if err := validateCommand(command); err != nil {
			return nil, err
		}
		req.Command = &command
	}
	if in.Schedule != nil {
		schedule := strings.TrimSpace(*in.Schedule)
		if err := s.ValidateSchedule(schedule); err != nil {
			return nil, err
		}
		req.Schedule = &schedule
	}

	// Recompute the next occurrence when anything that drives the timer moved.
	newSchedule := task.Schedule
	if req.Schedule != nil {
		newSchedule = *req.Schedule
	}
	newEnabled := task.Enabled
	if in.Enabled != nil {
		newEnabled = *in.Enabled
	}
	scheduleChanged := req.Schedule != nil || in.Enabled != nil
	if scheduleChanged {
		if newEnabled {
			next, err := s.nextRun(newSchedule, time.Now().UTC())
			if err != nil {
				return nil, err
			}
			// NextRunAt is a pointer-to-pointer so the patch can tell "leave it
			// alone" (nil) from "set it" (&value) from "clear it" (&nil).
			nextRunAt := &next
			req.NextRunAt = &nextRunAt
		} else {
			// Disabled tasks carry no pending occurrence; clearing next_run_at
			// also keeps the due-scan index free of dead rows.
			var clear *time.Time
			req.NextRunAt = &clear
		}
	}
	req.Enabled = &newEnabled

	updated, err := s.store.UpdateScheduledTask(ctx, taskID, req)
	if err != nil {
		return nil, fmt.Errorf("update scheduled task: %w", err)
	}
	return &updated, nil
}

// EnableTask and DisableTask are thin wrappers over UpdateTask so the
// next-run recalculation lives in exactly one place.
func (s *Service) EnableTask(ctx context.Context, serverID, taskID string) (*ScheduledTask, error) {
	enabled := true
	return s.UpdateTask(ctx, serverID, taskID, UpdateInput{Enabled: &enabled})
}

func (s *Service) DisableTask(ctx context.Context, serverID, taskID string) (*ScheduledTask, error) {
	enabled := false
	return s.UpdateTask(ctx, serverID, taskID, UpdateInput{Enabled: &enabled})
}

// DeleteTask removes the task and (via FK cascade) its run history.
func (s *Service) DeleteTask(ctx context.Context, serverID, taskID string) error {
	if _, err := s.GetTaskForServer(ctx, serverID, taskID); err != nil {
		return err
	}
	if err := s.store.DeleteScheduledTask(ctx, taskID); err != nil {
		return fmt.Errorf("delete scheduled task: %w", err)
	}
	return nil
}

// RunTask dispatches the task's command to Beacon now, records a TaskRun and
// stamps last_run_at. It does not shift the cron cycle — a manual run is an
// extra execution, mirroring Dokploy's execute() and `dokku cron:run`.
func (s *Service) RunTask(ctx context.Context, taskID string) (*TaskRun, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.dispatch(ctx, *task)
}

// dispatch performs one execution attempt and persists its TaskRun. The
// scheduler rolls next_run_at forward itself (before calling this) so a slow
// or failed dispatch can never make the same occurrence fire twice.
func (s *Service) dispatch(ctx context.Context, task ScheduledTask) (*TaskRun, error) {
	if s.dispatcher == nil {
		return nil, fmt.Errorf("%w: daemon client unavailable", ErrDispatch)
	}
	target, err := s.store.ServerControlTarget(ctx, task.ServerID)
	if err != nil {
		return nil, fmt.Errorf("resolve server %s control target: %w", task.ServerID, err)
	}

	run, err := s.store.CreateTaskRun(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("record task run: %w", err)
	}

	dispatchCtx, cancel := context.WithTimeout(ctx, dispatchTimeout)
	defer cancel()
	output, daemonErr := s.dispatcher.SendCommandWithOutput(dispatchCtx, target.NodeURL, target.NodeToken, target.ServerID, task.Command)

	status := store.TaskRunStatusSuccess
	exitCode := 0
	if daemonErr != nil {
		status = store.TaskRunStatusFailed
		exitCode = 1
		// Beacon's console channel writes stdin and answers {"ok":...}; it
		// cannot surface the in-container process exit code yet. Transport
		// failure is therefore the only failure signal we have, and the
		// daemon response body (when any) is kept as the run output.
		if strings.TrimSpace(output) == "" {
			output = daemonErr.Error()
		} else {
			output = strings.TrimSpace(output) + "\n" + daemonErr.Error()
		}
	}
	if len(output) > 64*1024 {
		output = output[:64*1024] + "\n[output truncated]"
	}

	// Always record the outcome, even when the caller's context is being torn
	// down mid-shutdown: history must not silently vanish.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err := s.store.CompleteTaskRun(finishCtx, run.ID, status, exitCode, output); err != nil {
		s.logger.Error("failed to persist scheduled task run", "task_id", task.ID, "run_id", run.ID, "error", err)
	}
	if err := s.store.UpdateScheduledTaskRunMeta(finishCtx, task.ID, &run.StartedAt, nil); err != nil {
		s.logger.Error("failed to stamp last run", "task_id", task.ID, "error", err)
	}

	completed, err := s.store.GetTaskRun(finishCtx, run.ID)
	if err != nil {
		return &run, fmt.Errorf("task dispatched but run record unreadable: %w", err)
	}
	if daemonErr != nil {
		return &completed, fmt.Errorf("%w: %v", ErrDispatch, daemonErr)
	}
	return &completed, nil
}

// ListRuns returns the most recent executions for a task, newest first.
func (s *Service) ListRuns(ctx context.Context, serverID, taskID string, limit int) ([]TaskRun, error) {
	if _, err := s.GetTaskForServer(ctx, serverID, taskID); err != nil {
		return nil, err
	}
	runs, err := s.store.ListTaskRuns(ctx, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list task runs: %w", err)
	}
	return runs, nil
}

// notFoundOr keeps store lookup misses in the ErrNotFound family while
// surfacing genuine database errors unchanged.
func notFoundOr(err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// pgx returns ErrNoRows for misses; anything else (constraint violations,
	// connection problems, bad uuid casts) must bubble up as a real error.
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
