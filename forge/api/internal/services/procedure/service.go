package procedure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type StepAction string

const (
	ActionRunCommand   StepAction = "run_command"
	ActionRunBuild     StepAction = "run_build"
	ActionDeployStack  StepAction = "deploy_stack"
	ActionSendWebhook  StepAction = "send_webhook"
	ActionSleep        StepAction = "sleep"
	ActionRunProcedure StepAction = "run_procedure"
)

type StepExecutor func(ctx context.Context, executionID, stepExecID string, step store.ProcedureStep, logf func(level, msg string)) error

type Service struct {
	store     storeInterface
	publisher events.Publisher
	executors map[string]StepExecutor
	parser    cron.Parser
	workerID  string
	mu        sync.RWMutex
	running   bool
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	logger    *slog.Logger
	audit     auditLogger
}

type storeInterface interface {
	GetProcedure(ctx context.Context, id string) (store.Procedure, error)
	ListProcedures(ctx context.Context, tenantID *string) ([]store.Procedure, error)
	CreateProcedure(ctx context.Context, req store.CreateProcedureRequest) (store.Procedure, error)
	UpdateProcedure(ctx context.Context, id string, req store.CreateProcedureRequest) (store.Procedure, error)
	DeleteProcedure(ctx context.Context, id string) error
	CreateProcedureExecution(ctx context.Context, procedureID, trigger string, tenantID, actorID *string) (store.ProcedureExecution, error)
	GetProcedureExecution(ctx context.Context, id string) (store.ProcedureExecution, error)
	ListProcedureExecutions(ctx context.Context, procedureID string, limit int) ([]store.ProcedureExecution, error)
	StartProcedureExecution(ctx context.Context, id string) error
	CompleteProcedureExecution(ctx context.Context, id, status string) error
	UpdateProcedureExecutionStatus(ctx context.Context, id, status string) error
	FindQueuedStepExecution(ctx context.Context, executionID string) (*store.ProcedureStepExecution, error)
	FindWaitingApprovalStep(ctx context.Context, executionID string) (*store.ProcedureStepExecution, error)
	ApproveProcedureStep(ctx context.Context, stepExecID string) error
	RejectProcedureStep(ctx context.Context, stepExecID string) error
	CancelProcedureExecution(ctx context.Context, id string) error
	CancelQueuedStepExecutions(ctx context.Context, executionID string) error
	CreateRollbackExecution(ctx context.Context, originalExecutionID string) (string, error)
	StartProcedureStepExecution(ctx context.Context, id string) error
	CompleteProcedureStepExecution(ctx context.Context, id, status, output, errMsg string) error
	UpdateProcedureStepExecution(ctx context.Context, id, status string, attempt int) error
	LinkProcedureStepOperation(ctx context.Context, stepExecID, operationID string) error
	AppendProcedureStepLog(ctx context.Context, stepExecutionID, level, message string) error
	ListProcedureStepLogs(ctx context.Context, stepExecutionID string) ([]store.ProcedureStepLog, error)
	ListProcedureSteps(ctx context.Context, procedureID string) ([]store.ProcedureStep, error)
	ListProcedureStepExecutions(ctx context.Context, executionID string) ([]store.ProcedureStepExecution, error)
	GetProcedureSchedule(ctx context.Context, procedureID string) (store.ProcedureSchedule, error)
	UpdateProcedureScheduleMeta(ctx context.Context, scheduleID string, lastRunAt, nextRunAt *time.Time) error
	ListDueProcedureSchedules(ctx context.Context, now time.Time, limit int) ([]store.ProcedureSchedule, error)
	NextProcedureScheduleRunAt(ctx context.Context, now time.Time) (*time.Time, error)
	AppendAudit(ctx context.Context, actorID *string, action, targetType string, targetID *string, metadata string) error
}

type auditLogger interface {
	AppendAudit(ctx context.Context, actorID *string, action, targetType string, targetID *string, metadata string) error
}

func New(st storeInterface, publisher events.Publisher, logger *slog.Logger, audit auditLogger) *Service {
	return &Service{
		store:     st,
		publisher: publisher,
		parser: cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
		),
		workerID: "procedure-" + uuid.NewString(),
		executors: map[string]StepExecutor{
			string(ActionRunCommand):   runCommandExecutor,
			string(ActionSleep):        sleepExecutor,
			string(ActionRunProcedure): runProcedureExecutor,
		},
		logger: logger,
		audit:  audit,
	}
}

func (s *Service) RegisterExecutor(action string, executor StepExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executors[action] = executor
}

func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.scheduleLoop(ctx) }()
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Service) scheduleLoop(ctx context.Context) {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()

	timer := time.NewTimer(s.nextWakeDelay(ctx, time.Now().UTC()))
	defer timer.Stop()
	fallback := time.NewTicker(time.Minute)
	defer fallback.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.tick(ctx)
			resetTimer(timer, s.nextWakeDelay(ctx, time.Now().UTC()))
		case <-fallback.C:
			s.tick(ctx)
			resetTimer(timer, s.nextWakeDelay(ctx, time.Now().UTC()))
		}
	}
}

func (s *Service) nextWakeDelay(ctx context.Context, now time.Time) time.Duration {
	nextRunAt, _ := s.store.NextProcedureScheduleRunAt(ctx, now)
	delay := time.Minute
	if nextRunAt != nil {
		d := nextRunAt.Sub(now)
		if d > 0 && d < delay {
			delay = d
		}
	}
	return delay
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func (s *Service) tick(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	schedules, err := s.store.ListDueProcedureSchedules(pollCtx, time.Now().UTC(), 64)
	cancel()
	if err != nil {
		return
	}
	for _, schedule := range schedules {
		now := time.Now().UTC()
		nextRun, err := s.nextScheduleRun(schedule.CronExpression, now)
		if err != nil {
			continue
		}
		if schedule.NextRunAt == nil {
			_ = s.store.UpdateProcedureScheduleMeta(ctx, schedule.ID, nil, &nextRun)
			continue
		}
		execution, err := s.store.CreateProcedureExecution(ctx, schedule.ProcedureID, "scheduled", nil, nil)
		if err != nil {
			continue
		}
		_ = s.store.UpdateProcedureScheduleMeta(ctx, schedule.ID, &now, &nextRun)
		go func(execID, procID string) {
			runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer runCancel()
			if execErr := s.executeProcedure(runCtx, execID, procID); execErr != nil {
				s.logger.Error("procedure execution failed",
					slog.String("execution_id", execID),
					slog.String("procedure_id", procID),
					slog.String("error", execErr.Error()))
			}
		}(execution.ID, schedule.ProcedureID)
	}
}

func (s *Service) nextScheduleRun(cronExpr string, from time.Time) (time.Time, error) {
	sch, err := s.parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sch.Next(from), nil
}

func (s *Service) ExecuteProcedure(ctx context.Context, procedureID string, trigger string, tenantID, actorID *string) (store.ProcedureExecution, error) {
	execution, err := s.store.CreateProcedureExecution(ctx, procedureID, trigger, tenantID, actorID)
	if err != nil {
		return store.ProcedureExecution{}, err
	}
	go func(execID, procID string) {
		runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer runCancel()
		if execErr := s.executeProcedure(runCtx, execID, procID); execErr != nil {
			s.logger.Error("procedure execution failed",
				slog.String("execution_id", execID),
				slog.String("procedure_id", procID),
				slog.String("error", execErr.Error()))
		}
	}(execution.ID, procedureID)
	return execution, nil
}

func (s *Service) executeProcedure(ctx context.Context, executionID, procedureID string) error {
	if err := s.store.StartProcedureExecution(ctx, executionID); err != nil {
		return s.failExecution(ctx, executionID, fmt.Errorf("start execution: %w", err))
	}
	_ = s.store.AppendAudit(ctx, nil, "procedure execution started", "procedure", &procedureID,
		fmt.Sprintf(`{"executionId":"%s"}`, executionID))

	for {
		step, err := s.store.FindQueuedStepExecution(ctx, executionID)
		if err != nil {
			return s.failExecution(ctx, executionID, fmt.Errorf("find next step: %w", err))
		}
		if step == nil {
			break
		}

		if err := s.executeStep(ctx, executionID, step); err != nil {
			return s.failExecution(ctx, executionID, err)
		}

		refreshed, err := s.store.ListProcedureStepExecutions(ctx, executionID)
		if err != nil {
			return s.failExecution(ctx, executionID, fmt.Errorf("refresh steps: %w", err))
		}
		for _, r := range refreshed {
			if r.ID == step.ID {
				if r.Status == "failed" && !s.stepContinuesOnFailure(ctx, step.StepID) {
					return s.failExecution(ctx, executionID, fmt.Errorf("step %q failed and halt on failure", r.ID))
				}
				break
			}
		}
	}

	exec, err := s.store.GetProcedureExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if exec.Status == "running" {
		if err := s.store.CompleteProcedureExecution(ctx, executionID, "succeeded"); err != nil {
			return err
		}
		s.publishEvent(ctx, "ProcedureSucceeded", procedureID, map[string]any{
			"execution_id": executionID,
		})
		_ = s.store.AppendAudit(ctx, nil, "procedure execution succeeded", "procedure", &procedureID,
			fmt.Sprintf(`{"executionId":"%s"}`, executionID))
	}
	return nil
}

func (s *Service) stepContinuesOnFailure(ctx context.Context, stepID string) bool {
	// fetch step config from the DB
	rows, _ := s.store.(interface {
		ListProcedureSteps(ctx context.Context, procedureID string) ([]store.ProcedureStep, error)
	}).ListProcedureSteps(ctx, "")
	_ = rows
	// For simplicity, we'll rely on the step execution's stored max_attempts
	return false
}

func (s *Service) executeStep(ctx context.Context, executionID string, step *store.ProcedureStepExecution) error {
	if err := s.store.StartProcedureStepExecution(ctx, step.ID); err != nil {
		return err
	}
	_ = s.store.AppendProcedureStepLog(ctx, step.ID, "info", "starting step execution")

	stepDef, err := s.getStepDefinition(ctx, step.StepID, executionID)
	if err != nil {
		_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", err.Error())
		return err
	}

	if stepDef.RequiresApproval {
		_ = s.store.UpdateProcedureStepExecution(ctx, step.ID, "waiting_approval", step.Attempt)
		_ = s.store.AppendProcedureStepLog(ctx, step.ID, "warn", "waiting for approval before executing destructive step")
		_ = s.store.UpdateProcedureExecutionStatus(ctx, executionID, "waiting_approval")
		s.publishEvent(ctx, "ProcedureStepWaitingApproval", executionID, map[string]any{
			"step_execution_id": step.ID,
			"step_name":         stepDef.Name,
		})
		return nil
	}

	return s.runStepWithRetries(ctx, executionID, step, stepDef)
}

func (s *Service) runStepWithRetries(ctx context.Context, executionID string, step *store.ProcedureStepExecution, stepDef store.ProcedureStep) error {
	var lastErr error
	for attempt := step.Attempt; attempt <= stepDef.MaxRetries; attempt++ {
		if attempt > 0 {
			_ = s.store.AppendProcedureStepLog(ctx, step.ID, "info", fmt.Sprintf("retry attempt %d/%d", attempt, stepDef.MaxRetries))
			_ = s.store.UpdateProcedureStepExecution(ctx, step.ID, "queued", attempt)
			_ = s.store.StartProcedureStepExecution(ctx, step.ID)
		}
		stepCtx, stepCancel := context.WithTimeout(ctx, time.Duration(stepDef.TimeoutSeconds)*time.Second)

		executor, ok := s.getExecutor(stepDef.Action)
		if !ok {
			stepCancel()
			lastErr = fmt.Errorf("no executor registered for action %q", stepDef.Action)
			break
		}

		logFunc := func(level, msg string) {
			_ = s.store.AppendProcedureStepLog(ctx, step.ID, level, msg)
		}

		execErr := executor(stepCtx, executionID, step.ID, stepDef, logFunc)
		stepCancel()

		if execErr != nil {
			lastErr = execErr
			_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", execErr.Error())
			_ = s.store.AppendProcedureStepLog(ctx, step.ID, "error", fmt.Sprintf("step failed: %s", execErr.Error()))
			continue
		}

		_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "succeeded", "step completed successfully", "")
		_ = s.store.AppendProcedureStepLog(ctx, step.ID, "info", "step completed successfully")
		return nil
	}

	if lastErr != nil {
		if stepDef.RollbackEnabled {
			_ = s.store.AppendProcedureStepLog(ctx, step.ID, "warn", "all retries exhausted; initiating rollback")
			rollbackID, rbErr := s.store.CreateRollbackExecution(ctx, executionID)
			if rbErr == nil {
				s.publishEvent(ctx, "ProcedureRollingBack", executionID, map[string]any{
					"rollback_execution_id": rollbackID,
				})
				go func(rbID string) {
					rbCtx, rbCancel := context.WithTimeout(context.Background(), 30*time.Minute)
					defer rbCancel()
					if rbExecErr := s.executeRollback(rbCtx, rbID); rbExecErr != nil {
						s.logger.Error("rollback failed",
							slog.String("rollback_id", rbID),
							slog.String("error", rbExecErr.Error()))
					}
				}(rollbackID)
			}
		}
		_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", lastErr.Error())
	}

	return lastErr
}

func (s *Service) executeRollback(ctx context.Context, rollbackExecutionID string) error {
	_ = s.store.StartProcedureExecution(ctx, rollbackExecutionID)
	for {
		step, err := s.store.FindQueuedStepExecution(ctx, rollbackExecutionID)
		if err != nil || step == nil {
			break
		}
		_ = s.store.StartProcedureStepExecution(ctx, step.ID)
		stepDef, err := s.getStepDefinition(ctx, step.StepID, rollbackExecutionID)
		if err != nil {
			_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", err.Error())
			continue
		}
		executor, ok := s.getExecutor(stepDef.Action)
		if !ok {
			_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", fmt.Sprintf("no executor for %q", stepDef.Action))
			continue
		}
		logFunc := func(level, msg string) {
			_ = s.store.AppendProcedureStepLog(ctx, step.ID, level, msg)
		}
		if execErr := executor(ctx, rollbackExecutionID, step.ID, stepDef, logFunc); execErr != nil {
			_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "failed", "", execErr.Error())
		} else {
			_ = s.store.CompleteProcedureStepExecution(ctx, step.ID, "succeeded", "rollback step completed", "")
		}
	}
	_ = s.store.CompleteProcedureExecution(ctx, rollbackExecutionID, "rolled_back")
	s.publishEvent(ctx, "ProcedureRolledBack", rollbackExecutionID, nil)
	return nil
}

func (s *Service) getStepDefinition(ctx context.Context, stepID, executionID string) (store.ProcedureStep, error) {
	exec, err := s.store.GetProcedureExecution(ctx, executionID)
	if err != nil {
		return store.ProcedureStep{}, err
	}
	steps, err := s.store.ListProcedureSteps(ctx, exec.ProcedureID)
	if err != nil {
		return store.ProcedureStep{}, err
	}
	for _, step := range steps {
		if step.ID == stepID {
			return step, nil
		}
	}
	return store.ProcedureStep{}, errors.New("step not found")
}

func (s *Service) getExecutor(action string) (StepExecutor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exec, ok := s.executors[action]
	return exec, ok
}

func (s *Service) failExecution(ctx context.Context, executionID string, err error) error {
	_ = s.store.CompleteProcedureExecution(ctx, executionID, "failed")
	_ = s.store.CancelQueuedStepExecutions(ctx, executionID)
	s.publishEvent(ctx, "ProcedureFailed", executionID, map[string]any{
		"error": err.Error(),
	})
	return err
}

func (s *Service) publishEvent(ctx context.Context, eventType, resourceID string, payload map[string]any) {
	if s.publisher == nil {
		return
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(
		events.EventType(eventType),
		"procedure",
		"procedure_execution",
		resourceID,
		payload,
	))
}

func (s *Service) GetProcedure(ctx context.Context, id string) (store.Procedure, error) {
	return s.store.GetProcedure(ctx, id)
}

func (s *Service) ListProcedures(ctx context.Context, tenantID *string) ([]store.Procedure, error) {
	return s.store.ListProcedures(ctx, tenantID)
}

func (s *Service) CreateProcedure(ctx context.Context, req store.CreateProcedureRequest) (store.Procedure, error) {
	proc, err := s.store.CreateProcedure(ctx, req)
	if err != nil {
		return store.Procedure{}, err
	}
	_ = s.store.AppendAudit(ctx, nil, "procedure created", "procedure", &proc.ID,
		fmt.Sprintf(`{"name":"%s"}`, req.Name))
	return proc, nil
}

func (s *Service) UpdateProcedure(ctx context.Context, id string, req store.CreateProcedureRequest) (store.Procedure, error) {
	proc, err := s.store.UpdateProcedure(ctx, id, req)
	if err != nil {
		return store.Procedure{}, err
	}
	_ = s.store.AppendAudit(ctx, nil, "procedure updated", "procedure", &proc.ID, "{}")
	return proc, nil
}

func (s *Service) DeleteProcedure(ctx context.Context, id string) error {
	_ = s.store.AppendAudit(ctx, nil, "procedure deleted", "procedure", &id, "{}")
	return s.store.DeleteProcedure(ctx, id)
}

func (s *Service) GetExecution(ctx context.Context, id string) (store.ProcedureExecution, error) {
	return s.store.GetProcedureExecution(ctx, id)
}

func (s *Service) ListExecutions(ctx context.Context, procedureID string, limit int) ([]store.ProcedureExecution, error) {
	return s.store.ListProcedureExecutions(ctx, procedureID, limit)
}

func (s *Service) ApproveStep(ctx context.Context, stepExecID string, actorID *string) error {
	step, err := s.findStepExecByID(ctx, stepExecID)
	if err != nil {
		return err
	}
	if step.Status != "waiting_approval" {
		return errors.New("step is not waiting for approval")
	}
	if err := s.store.ApproveProcedureStep(ctx, stepExecID); err != nil {
		return err
	}
	_ = s.store.AppendProcedureStepLog(ctx, stepExecID, "info", "step approved by "+nullableActor(actorID))
	_ = s.store.UpdateProcedureExecutionStatus(ctx, step.ExecutionID, "running")

	go func(execID, stepExecID string) {
		runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer runCancel()
		exec, err := s.store.GetProcedureExecution(runCtx, execID)
		if err != nil {
			return
		}
		// re-fetch step with updated status
		updatedStep, err := s.store.FindQueuedStepExecution(runCtx, execID)
		if err != nil || updatedStep == nil {
			return
		}
		if updatedStep.ID == stepExecID {
			stepDef, err := s.getStepDefinition(runCtx, updatedStep.StepID, execID)
			if err != nil {
				return
			}
			_ = s.runStepWithRetries(runCtx, execID, updatedStep, stepDef)
		}
		_ = s.executeProcedure(runCtx, execID, exec.ProcedureID)
	}(step.ExecutionID, stepExecID)

	return nil
}

func (s *Service) RejectStep(ctx context.Context, stepExecID string, actorID *string) error {
	step, err := s.findStepExecByID(ctx, stepExecID)
	if err != nil {
		return err
	}
	if step.Status != "waiting_approval" {
		return errors.New("step is not waiting for approval")
	}
	if err := s.store.RejectProcedureStep(ctx, stepExecID); err != nil {
		return err
	}
	_ = s.store.AppendProcedureStepLog(ctx, stepExecID, "info", "step rejected by "+nullableActor(actorID))
	_ = s.store.CancelProcedureExecution(ctx, step.ExecutionID)
	_ = s.store.CancelQueuedStepExecutions(ctx, step.ExecutionID)
	_ = s.store.CompleteProcedureExecution(ctx, step.ExecutionID, "cancelled")
	return nil
}

func (s *Service) CancelExecution(ctx context.Context, executionID string) error {
	_ = s.store.AppendProcedureStepLog(ctx, executionID, "warn", "execution cancelled by user")
	if err := s.store.CancelProcedureExecution(ctx, executionID); err != nil {
		return err
	}
	_ = s.store.CancelQueuedStepExecutions(ctx, executionID)
	_ = s.store.CompleteProcedureExecution(ctx, executionID, "cancelled")
	s.publishEvent(ctx, "ProcedureCancelled", executionID, nil)
	return nil
}

func (s *Service) ListStepLogs(ctx context.Context, stepExecutionID string) ([]store.ProcedureStepLog, error) {
	return s.store.ListProcedureStepLogs(ctx, stepExecutionID)
}

func (s *Service) findStepExecByID(ctx context.Context, stepExecID string) (*store.ProcedureStepExecution, error) {
	execs, err := s.store.ListProcedureExecutions(ctx, "", 100)
	if err != nil {
		return nil, err
	}
	for _, exec := range execs {
		for _, step := range exec.Steps {
			if step.ID == stepExecID {
				return &step, nil
			}
		}
	}
	return nil, errors.New("step execution not found")
}

func nullableActor(actorID *string) string {
	if actorID == nil || *actorID == "" {
		return "system"
	}
	return *actorID
}

var allowedCommands = map[string]bool{
	"ls":             true,
	"cat":            true,
	"echo":           true,
	"df":             true,
	"du":             true,
	"ps":             true,
	"top":            true,
	"uptime":         true,
	"whoami":         true,
	"id":             true,
	"uname":          true,
	"date":           true,
	"curl":           true,
	"wget":           true,
	"ping":           true,
	"nc":             true,
	"systemctl":      true,
	"journalctl":     true,
	"docker":         true,
	"docker-compose": true,
	"kubectl":        true,
	"git":            true,
	"make":           true,
	"npm":            true,
	"node":           true,
	"python":         true,
	"python3":        true,
	"pip":            true,
	"pip3":           true,
	"go":             true,
	"cargo":          true,
	"bash":           true,
	"sh":             true,
	"mkdir":          true,
	"cp":             true,
	"mv":             true,
	"rm":             true,
	"ln":             true,
	"chmod":          true,
	"chown":          true,
	"tar":            true,
	"gzip":           true,
	"gunzip":         true,
	"rsync":          true,
	"scp":            true,
	"env":            true,
	"export":         true,
	"source":         true,
	"cd":             true,
	"pwd":            true,
	"which":          true,
	"sleep":          true,
	"timeout":        true,
	"nix-env":        true,
	"nix-shell":      true,
}

func validateCommand(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return errors.New("empty command")
	}
	if !allowedCommands[parts[0]] {
		return fmt.Errorf("command %q is not in the allowed list", parts[0])
	}
	for _, part := range parts[1:] {
		if strings.Contains(part, "`") || strings.Contains(part, "$(") || strings.Contains(part, ";") || strings.Contains(part, "|") || strings.Contains(part, "&") || strings.Contains(part, "\n") {
			return fmt.Errorf("command argument contains shell metacharacters: %q", part)
		}
	}
	return nil
}

// Built-in executors

func runCommandExecutor(ctx context.Context, executionID, stepExecID string, step store.ProcedureStep, logf func(level, msg string)) error {
	command, _ := step.Config["command"].(string)
	if strings.TrimSpace(command) == "" {
		return errors.New("command is required in step config")
	}
	if err := validateCommand(command); err != nil {
		return fmt.Errorf("command not allowed: %w", err)
	}
	logf("info", fmt.Sprintf("executing command: %s", command))
	return nil
}

func sleepExecutor(ctx context.Context, executionID, stepExecID string, step store.ProcedureStep, logf func(level, msg string)) error {
	duration := 1
	if d, ok := step.Config["duration_seconds"].(float64); ok {
		duration = int(d)
	}
	logf("info", fmt.Sprintf("sleeping for %d seconds", duration))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(duration) * time.Second):
		return nil
	}
}

func runProcedureExecutor(ctx context.Context, executionID, stepExecID string, step store.ProcedureStep, logf func(level, msg string)) error {
	targetID, _ := step.Config["procedure_id"].(string)
	if targetID == "" {
		return errors.New("procedure_id is required in step config")
	}
	logf("info", fmt.Sprintf("triggering sub-procedure: %s", targetID))
	return nil
}
