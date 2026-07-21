package procedure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type fakeStore struct {
	mu              sync.Mutex
	procedures      map[string]store.Procedure
	steps           map[string][]store.ProcedureStep
	executions      map[string]store.ProcedureExecution
	stepExecs       map[string][]store.ProcedureStepExecution
	schedules       map[string]store.ProcedureSchedule
	logs            map[string][]store.ProcedureStepLog
	auditEvents     []string
	stepDefinitions map[string]store.ProcedureStep
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		procedures:      make(map[string]store.Procedure),
		steps:           make(map[string][]store.ProcedureStep),
		executions:      make(map[string]store.ProcedureExecution),
		stepExecs:       make(map[string][]store.ProcedureStepExecution),
		schedules:       make(map[string]store.ProcedureSchedule),
		logs:            make(map[string][]store.ProcedureStepLog),
		stepDefinitions: make(map[string]store.ProcedureStep),
	}
}

func (f *fakeStore) GetProcedure(ctx context.Context, id string) (store.Procedure, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.procedures[id]
	if !ok {
		return store.Procedure{}, errors.New("not found")
	}
	steps := f.steps[id]
	if steps != nil {
		p.Steps = steps
	}
	return p, nil
}

func (f *fakeStore) ListProcedures(ctx context.Context, tenantID *string) ([]store.Procedure, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []store.Procedure
	for _, p := range f.procedures {
		result = append(result, p)
	}
	return result, nil
}

func (f *fakeStore) CreateProcedure(ctx context.Context, req store.CreateProcedureRequest) (store.Procedure, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("proc-%d", len(f.procedures)+1)
	p := store.Procedure{
		ID: id, Name: req.Name, Description: req.Description,
		TenantID: req.TenantID, Enabled: req.Enabled,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.procedures[id] = p
	f.steps[id] = nil
	for _, s := range req.Steps {
		stepID := fmt.Sprintf("step-%d", len(f.steps[id])+1)
		step := store.ProcedureStep{
			ID: stepID, ProcedureID: id, Position: s.Position,
			Name: s.Name, Action: s.Action, Config: s.Config,
			MaxRetries: s.MaxRetries, TimeoutSeconds: s.TimeoutSeconds,
			RequiresApproval: s.RequiresApproval, ContinueOnFailure: s.ContinueOnFailure,
			RollbackEnabled: s.RollbackEnabled,
		}
		f.steps[id] = append(f.steps[id], step)
		f.stepDefinitions[stepID] = step
	}
	return p, nil
}

func (f *fakeStore) UpdateProcedure(ctx context.Context, id string, req store.CreateProcedureRequest) (store.Procedure, error) {
	return f.CreateProcedure(ctx, req)
}

func (f *fakeStore) DeleteProcedure(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.procedures, id)
	return nil
}

func (f *fakeStore) CreateProcedureExecution(ctx context.Context, procedureID, trigger string, tenantID, actorID *string) (store.ProcedureExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("exec-%d", len(f.executions)+1)
	_, ok := f.procedures[procedureID]
	if !ok {
		return store.ProcedureExecution{}, errors.New("procedure not found")
	}
	exec := store.ProcedureExecution{
		ID: id, ProcedureID: procedureID, Status: "queued", Trigger: trigger,
		TenantID: tenantID, ActorID: actorID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.executions[id] = exec
	steps := f.steps[procedureID]
	var stepExecs []store.ProcedureStepExecution
	for _, step := range steps {
		seID := fmt.Sprintf("se-%s-%d", id, step.Position)
		se := store.ProcedureStepExecution{
			ID: seID, ExecutionID: id, StepID: step.ID, Position: step.Position,
			Status: "queued", Attempt: 0, MaxAttempts: step.MaxRetries,
		}
		stepExecs = append(stepExecs, se)
		f.stepDefinitions[seID] = step
	}
	f.stepExecs[id] = stepExecs
	exec.Steps = stepExecs
	return exec, nil
}

func (f *fakeStore) GetProcedureExecution(ctx context.Context, id string) (store.ProcedureExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[id]
	if !ok {
		return store.ProcedureExecution{}, errors.New("not found")
	}
	exec.Steps = f.stepExecs[id]
	return exec, nil
}

func (f *fakeStore) ListProcedureExecutions(ctx context.Context, procedureID string, limit int) ([]store.ProcedureExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []store.ProcedureExecution
	for _, e := range f.executions {
		if e.ProcedureID == procedureID || procedureID == "" {
			e.Steps = f.stepExecs[e.ID]
			result = append(result, e)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeStore) StartProcedureExecution(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[id]
	if ok {
		exec.Status = "running"
		now := time.Now()
		exec.StartedAt = &now
		f.executions[id] = exec
	}
	return nil
}

func (f *fakeStore) CompleteProcedureExecution(ctx context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[id]
	if ok {
		exec.Status = status
		now := time.Now()
		exec.CompletedAt = &now
		f.executions[id] = exec
	}
	return nil
}

func (f *fakeStore) UpdateProcedureExecutionStatus(ctx context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[id]
	if ok {
		exec.Status = status
		f.executions[id] = exec
	}
	return nil
}

func (f *fakeStore) FindQueuedStepExecution(ctx context.Context, executionID string) (*store.ProcedureStepExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	steps := f.stepExecs[executionID]
	for _, s := range steps {
		if s.Status == "queued" {
			return &s, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) FindWaitingApprovalStep(ctx context.Context, executionID string) (*store.ProcedureStepExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	steps := f.stepExecs[executionID]
	for _, s := range steps {
		if s.Status == "waiting_approval" {
			return &s, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) ApproveProcedureStep(ctx context.Context, stepExecID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, steps := range f.stepExecs {
		for i, s := range steps {
			if s.ID == stepExecID && s.Status == "waiting_approval" {
				steps[i].Status = "queued"
				return nil
			}
		}
	}
	return errors.New("step not found")
}

func (f *fakeStore) RejectProcedureStep(ctx context.Context, stepExecID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, steps := range f.stepExecs {
		for i, s := range steps {
			if s.ID == stepExecID && s.Status == "waiting_approval" {
				steps[i].Status = "cancelled"
				return nil
			}
		}
	}
	return errors.New("step not found")
}

func (f *fakeStore) CancelProcedureExecution(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	exec, ok := f.executions[id]
	if ok {
		exec.Status = "cancelled"
		f.executions[id] = exec
	}
	return nil
}

func (f *fakeStore) CancelQueuedStepExecutions(ctx context.Context, executionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	steps := f.stepExecs[executionID]
	for i, s := range steps {
		if s.Status == "queued" || s.Status == "waiting_approval" {
			steps[i].Status = "cancelled"
		}
	}
	return nil
}

func (f *fakeStore) CreateRollbackExecution(ctx context.Context, originalExecutionID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	orig, ok := f.executions[originalExecutionID]
	if !ok {
		return "", errors.New("original execution not found")
	}
	rollbackID := fmt.Sprintf("rollback-%s", originalExecutionID)
	rbExec := store.ProcedureExecution{
		ID: rollbackID, ProcedureID: orig.ProcedureID,
		Status: "rolling_back", Trigger: "rollback",
	}
	f.executions[rollbackID] = rbExec
	steps := f.steps[orig.ProcedureID]
	var stepExecs []store.ProcedureStepExecution
	for _, step := range steps {
		if step.RollbackEnabled {
			seID := fmt.Sprintf("rb-se-%s-%d", rollbackID, step.Position)
			se := store.ProcedureStepExecution{
				ID: seID, ExecutionID: rollbackID, StepID: step.ID,
				Position: step.Position, Status: "queued", MaxAttempts: step.MaxRetries,
			}
			stepExecs = append(stepExecs, se)
			f.stepDefinitions[seID] = step
		}
	}
	f.stepExecs[rollbackID] = stepExecs
	return rollbackID, nil
}

func (f *fakeStore) StartProcedureStepExecution(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, steps := range f.stepExecs {
		for i, s := range steps {
			if s.ID == id && s.Status == "queued" {
				steps[i].Status = "running"
				now := time.Now()
				steps[i].StartedAt = &now
				return nil
			}
		}
	}
	return nil
}

func (f *fakeStore) CompleteProcedureStepExecution(ctx context.Context, id, status, output, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, steps := range f.stepExecs {
		for i, s := range steps {
			if s.ID == id {
				steps[i].Status = status
				steps[i].Output = output
				steps[i].Error = errMsg
				now := time.Now()
				steps[i].CompletedAt = &now
				return nil
			}
		}
	}
	return nil
}

func (f *fakeStore) UpdateProcedureStepExecution(ctx context.Context, id, status string, attempt int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, steps := range f.stepExecs {
		for i, s := range steps {
			if s.ID == id {
				steps[i].Status = status
				steps[i].Attempt = attempt
				return nil
			}
		}
	}
	return nil
}

func (f *fakeStore) LinkProcedureStepOperation(ctx context.Context, stepExecID, operationID string) error { return nil }

func (f *fakeStore) AppendProcedureStepLog(ctx context.Context, stepExecutionID, level, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs[stepExecutionID] = append(f.logs[stepExecutionID], store.ProcedureStepLog{
		ID: fmt.Sprintf("log-%d", len(f.logs[stepExecutionID])+1),
		StepExecutionID: stepExecutionID,
		Level: level, Message: message, CreatedAt: time.Now(),
	})
	return nil
}

func (f *fakeStore) ListProcedureStepLogs(ctx context.Context, stepExecutionID string) ([]store.ProcedureStepLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[stepExecutionID], nil
}

func (f *fakeStore) ListProcedureSteps(ctx context.Context, procedureID string) ([]store.ProcedureStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.steps[procedureID], nil
}

func (f *fakeStore) ListProcedureStepExecutions(ctx context.Context, executionID string) ([]store.ProcedureStepExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stepExecs[executionID], nil
}

func (f *fakeStore) GetProcedureSchedule(ctx context.Context, procedureID string) (store.ProcedureSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.schedules[procedureID]
	if !ok {
		return store.ProcedureSchedule{}, errors.New("not found")
	}
	return s, nil
}

func (f *fakeStore) UpdateProcedureScheduleMeta(ctx context.Context, scheduleID string, lastRunAt, nextRunAt *time.Time) error { return nil }

func (f *fakeStore) ListDueProcedureSchedules(ctx context.Context, now time.Time, limit int) ([]store.ProcedureSchedule, error) {
	return nil, nil
}

func (f *fakeStore) NextProcedureScheduleRunAt(ctx context.Context, now time.Time) (*time.Time, error) { return nil, nil }

func (f *fakeStore) AppendAudit(ctx context.Context, actorID *string, action, targetType string, targetID *string, metadata string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auditEvents = append(f.auditEvents, action)
	return nil
}

type fakePublisher struct{}

func (f *fakePublisher) Publish(ctx context.Context, envelope events.Envelope) error { return nil }

func testSvc(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	fakeSt := newFakeStore()
	svc := New(fakeSt, &fakePublisher{}, slog.Default(), fakeSt)
	svc.RegisterExecutor("test_succeed", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		logf("info", "test step succeeded")
		return nil
	})
	svc.RegisterExecutor("test_fail", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		logf("error", "test step failed")
		return errors.New("step failed intentionally")
	})
	svc.RegisterExecutor("test_timeout", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		<-ctx.Done()
		return ctx.Err()
	})
	return svc, fakeSt
}

func TestSequentialSuccess(t *testing.T) {
	svc, fs := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-proc",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step-1", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "step-2", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}
	for _, s := range result.Steps {
		if s.Status != "succeeded" {
			t.Fatalf("step %d expected succeeded, got %s", s.Position, s.Status)
		}
	}
	if len(fs.auditEvents) == 0 {
		t.Fatal("expected audit events")
	}
}

func TestStepFailure(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-fail",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step-1", Action: "test_fail", MaxRetries: 0, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "failed" {
		t.Fatalf("expected failed, got %s", result.Status)
	}
}

func TestRetry(t *testing.T) {
	svc, _ := testSvc(t)
	var attemptCount int
	svc.RegisterExecutor("retry_test", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		attemptCount++
		if attemptCount < 3 {
			return errors.New("transient failure")
		}
		return nil
	})
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-retry",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "retry-step", Action: "retry_test", MaxRetries: 3, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s (attempts: %d)", result.Status, attemptCount)
	}
}

func TestCancellation(t *testing.T) {
	svc, _ := testSvc(t)
	svc.RegisterExecutor("wait_forever", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-cancel",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "wait-step", Action: "wait_forever", MaxRetries: 1, TimeoutSeconds: 30},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	err = svc.CancelExecution(context.Background(), exec.ID)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", result.Status)
	}
}

func TestApprovalRequired(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-approval",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "safe-step", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "destructive-step", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5, RequiresApproval: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "waiting_approval" {
		t.Fatalf("expected waiting_approval, got %s", result.Status)
	}
	// Find the step waiting for approval
	for _, step := range result.Steps {
		if step.Status == "waiting_approval" {
			// Approve
			userID := "admin-user"
			if err := svc.ApproveStep(context.Background(), step.ID, &userID); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	time.Sleep(1500 * time.Millisecond)
	final, _ := svc.GetExecution(context.Background(), exec.ID)
	if final.Status != "succeeded" {
		t.Fatalf("expected succeeded after approval, got %s", final.Status)
	}
}

func TestUnauthorizedApproval(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-no-approval-needed",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "normal-step", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	// Try to approve a step that's not waiting for approval
	for _, step := range result.Steps {
		if step.Status == "succeeded" {
			err := svc.ApproveStep(context.Background(), step.ID, nil)
			if err == nil {
				t.Fatal("expected error when approving non-waiting step")
			}
			break
		}
	}
}

func TestScheduledExecution(t *testing.T) {
	svc, fs := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-scheduled",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step-1", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Execute manually to simulate scheduled trigger
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "scheduled", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s", result.Status)
	}
	if result.Trigger != "scheduled" {
		t.Fatalf("expected trigger 'scheduled', got %s", result.Trigger)
	}
	// Verify audit events contain execution started + succeeded
	foundStarted := false
	foundSucceeded := false
	for _, evt := range fs.auditEvents {
		if evt == "procedure execution started" {
			foundStarted = true
		}
		if evt == "procedure execution succeeded" {
			foundSucceeded = true
		}
	}
	if !foundStarted || !foundSucceeded {
		t.Fatal("expected audit events for execution lifecycle")
	}
}

func TestRollbackHook(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-rollback",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "deploy", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5, RollbackEnabled: true},
			{Position: 2, Name: "cleanup", Action: "test_fail", MaxRetries: 0, TimeoutSeconds: 5, RollbackEnabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "failed" {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	// Wait for rollback to complete
	time.Sleep(500 * time.Millisecond)
	executions, _ := svc.ListExecutions(context.Background(), proc.ID, 10)
	if len(executions) < 2 {
		t.Fatalf("expected at least 2 executions (original + rollback), got %d", len(executions))
	}
}

func TestDuplicateTrigger(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-duplicate",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step-1", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec1, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec2, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Both should succeed independently
	time.Sleep(500 * time.Millisecond)
	r1, _ := svc.GetExecution(context.Background(), exec1.ID)
	r2, _ := svc.GetExecution(context.Background(), exec2.ID)
	if r1.Status != "succeeded" {
		t.Fatalf("first execution expected succeeded, got %s", r1.Status)
	}
	if r2.Status != "succeeded" {
		t.Fatalf("second execution expected succeeded, got %s", r2.Status)
	}
}

func TestRestartAfterStepCompletion(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-restart",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "deploy", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "configure", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	// Simulate restart by calling execute again (duplicate execution)
	exec2, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	r1, _ := svc.GetExecution(context.Background(), exec.ID)
	r2, _ := svc.GetExecution(context.Background(), exec2.ID)
	if r1.Status != "succeeded" {
		t.Fatalf("first execution expected succeeded, got %s", r1.Status)
	}
	if r2.Status != "succeeded" {
		t.Fatalf("second execution expected succeeded, got %s", r2.Status)
	}
}

func TestValidateCommand(t *testing.T) {
	t.Run("allowed command", func(t *testing.T) {
		if err := validateCommand("ls -la"); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("disallowed command", func(t *testing.T) {
		if err := validateCommand("sudo rm -rf /"); err == nil {
			t.Fatal("expected error for disallowed command")
		}
	})

	t.Run("empty command", func(t *testing.T) {
		if err := validateCommand(""); err == nil {
			t.Fatal("expected error for empty command")
		}
	})

	t.Run("shell metacharacter backtick", func(t *testing.T) {
		if err := validateCommand("echo `whoami`"); err == nil {
			t.Fatal("expected error for shell metacharacter")
		}
	})

	t.Run("shell metacharacter subshell", func(t *testing.T) {
		if err := validateCommand("echo $(whoami)"); err == nil {
			t.Fatal("expected error for shell metacharacter")
		}
	})

	t.Run("shell metacharacter semicolon", func(t *testing.T) {
		if err := validateCommand("echo hello; ls"); err == nil {
			t.Fatal("expected error for shell metacharacter")
		}
	})

	t.Run("shell metacharacter pipe", func(t *testing.T) {
		if err := validateCommand("ls | grep foo"); err == nil {
			t.Fatal("expected error for shell metacharacter")
		}
	})
}

func TestStepLogs(t *testing.T) {
	svc, _ := testSvc(t)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "test-logs",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "log-step", Action: "test_succeed", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if len(result.Steps) > 0 {
		logs, err := svc.ListStepLogs(context.Background(), result.Steps[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) == 0 {
			t.Fatal("expected step logs")
		}
	}
}
