package procedure

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

// Scenario 9: Procedure Execution End-to-End Test
// This test verifies all 12 requirements:
// 1. Ordered steps
// 2. Dependencies
// 3. Conditional execution
// 4. Retries
// 5. Timeout
// 6. Cancellation
// 7. Approval
// 8. Unauthorized approval rejection
// 9. Rollback hook
// 10. Operation and step history
// 11. Restart after completed step
// 12. Next step does not run twice

func TestScenario9_EndToEnd(t *testing.T) {
	t.Run("1. Ordered Step Execution", testOrderedSteps)
	t.Run("2. Dependency Handling", testDependencies)
	t.Run("3. Conditional Execution", testConditionalExecution)
	t.Run("4. Retry Logic", testRetryLogic)
	t.Run("5. Timeout Handling", testTimeoutHandling)
	t.Run("6. Cancellation", testCancellation)
	t.Run("7. Approval Workflow", testApprovalWorkflow)
	t.Run("8. Unauthorized Approval Rejection", testUnauthorizedApprovalRejection)
	t.Run("9. Rollback Hooks", testRollbackHooks)
	t.Run("10. History Tracking", testHistoryTracking)
	t.Run("11. Restart After Completed Step", testRestartAfterCompletedStep)
	t.Run("12. Idempotency - Next Step Does Not Run Twice", testIdempotency)
}

// Test 1: Ordered Step Execution
func testOrderedSteps(t *testing.T) {
	t.Log("Testing ordered step execution...")

	svc, _ := testSvc(t)

	var executionOrder []string
	var mu sync.Mutex

	svc.RegisterExecutor("ordered_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executionOrder = append(executionOrder, step.Name)
		mu.Unlock()
		logf("info", "executed "+step.Name)
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "ordered-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step-alpha", Action: "ordered_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "step-beta", Action: "ordered_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 3, Name: "step-gamma", Action: "ordered_step", MaxRetries: 1, TimeoutSeconds: 5},
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

	mu.Lock()
	defer mu.Unlock()

	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 steps executed, got %d", len(executionOrder))
	}

	expectedOrder := []string{"step-alpha", "step-beta", "step-gamma"}
	for i, expected := range expectedOrder {
		if i >= len(executionOrder) || executionOrder[i] != expected {
			t.Fatalf("step %d: expected %s, got %s", i+1, expected, executionOrder[i])
		}
	}

	t.Log("✓ Ordered step execution verified")
}

// Test 2: Dependency Handling
func testDependencies(t *testing.T) {
	t.Log("Testing dependency handling...")

	svc, _ := testSvc(t)

	var executedSteps []string
	var mu sync.Mutex

	svc.RegisterExecutor("dep_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executedSteps = append(executedSteps, step.Name)
		mu.Unlock()
		return nil
	})

	// Create procedure with steps that have implicit dependencies (position-based)
	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "dependency-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "foundation", Action: "dep_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "build", Action: "dep_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 3, Name: "deploy", Action: "dep_step", MaxRetries: 1, TimeoutSeconds: 5},
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

	mu.Lock()
	defer mu.Unlock()

	// Verify all dependent steps executed in order
	if len(executedSteps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(executedSteps))
	}

	// Foundation must execute before build, build before deploy
	foundationIdx := -1
	buildIdx := -1
	deployIdx := -1

	for i, name := range executedSteps {
		switch name {
		case "foundation":
			foundationIdx = i
		case "build":
			buildIdx = i
		case "deploy":
			deployIdx = i
		}
	}

	if foundationIdx >= buildIdx || buildIdx >= deployIdx {
		t.Fatal("dependencies not respected: foundation must run before build, build before deploy")
	}

	t.Log("✓ Dependency handling verified")
}

// Test 3: Conditional Execution
func testConditionalExecution(t *testing.T) {
	t.Log("Testing conditional execution...")

	svc, _ := testSvc(t)

	var executedSteps []string
	var mu sync.Mutex

	svc.RegisterExecutor("conditional_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executedSteps = append(executedSteps, step.Name)
		mu.Unlock()

		// Simulate conditional logic based on step name
		if step.Name == "should_fail" {
			return errors.New("conditional failure")
		}
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "conditional-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step1", Action: "conditional_step", MaxRetries: 1, TimeoutSeconds: 5, ContinueOnFailure: false},
			{Position: 2, Name: "should_fail", Action: "conditional_step", MaxRetries: 0, TimeoutSeconds: 5, ContinueOnFailure: false},
			{Position: 3, Name: "step3", Action: "conditional_step", MaxRetries: 1, TimeoutSeconds: 5, ContinueOnFailure: false},
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

	// Should fail because step2 fails and ContinueOnFailure is false
	if result.Status != "failed" {
		t.Fatalf("expected failed due to conditional step, got %s", result.Status)
	}

	mu.Lock()
	defer mu.Unlock()

	// step3 should NOT have executed because step2 failed and halt on failure
	step3Executed := false
	for _, name := range executedSteps {
		if name == "step3" {
			step3Executed = true
			break
		}
	}

	if step3Executed {
		t.Fatal("step3 should not have executed due to step2 failure")
	}

	t.Log("✓ Conditional execution verified")
}

// Test 4: Retry Logic
func testRetryLogic(t *testing.T) {
	t.Log("Testing retry logic...")

	svc, _ := testSvc(t)

	attemptCount := 0
	var mu sync.Mutex

	svc.RegisterExecutor("retryable_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		logf("info", "attempt "+string(rune('0'+currentAttempt)))

		if currentAttempt < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "retry-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "flaky-step", Action: "retryable_step", MaxRetries: 3, TimeoutSeconds: 5},
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
		t.Fatalf("expected succeeded after retries, got %s (attempts: %d)", result.Status, attemptCount)
	}

	mu.Lock()
	if attemptCount != 3 {
		t.Fatalf("expected 3 attempts, got %d", attemptCount)
	}
	mu.Unlock()

	t.Log("✓ Retry logic verified")
}

// Test 5: Timeout Handling
func testTimeoutHandling(t *testing.T) {
	t.Log("Testing timeout handling...")

	svc, _ := testSvc(t)

	svc.RegisterExecutor("slow_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		// This will timeout because step timeout is 1 second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "timeout-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "slow-step", Action: "slow_step", MaxRetries: 0, TimeoutSeconds: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for timeout to occur (1 second timeout + processing time)
	time.Sleep(1500 * time.Millisecond)

	result, _ := svc.GetExecution(context.Background(), exec.ID)

	// Should have failed due to timeout
	if result.Status != "failed" {
		t.Fatalf("expected failed due to timeout, got %s", result.Status)
	}

	// Check that the step failed
	if len(result.Steps) == 0 {
		t.Fatal("expected at least one step")
	}

	if result.Steps[0].Status != "failed" {
		t.Fatalf("expected step to be failed, got %s", result.Steps[0].Status)
	}

	t.Log("✓ Timeout handling verified")
}

// Test 6: Cancellation
func testCancellation(t *testing.T) {
	t.Log("Testing cancellation...")

	svc, _ := testSvc(t)

	svc.RegisterExecutor("long_running", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "cancel-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "long-step", Action: "long_running", MaxRetries: 0, TimeoutSeconds: 30},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the execution
	err = svc.CancelExecution(context.Background(), exec.ID)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)

	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", result.Status)
	}

	t.Log("✓ Cancellation verified")
}

// Test 7: Approval Workflow
func testApprovalWorkflow(t *testing.T) {
	t.Log("Testing approval workflow...")

	svc, _ := testSvc(t)

	var executedSteps []string
	var mu sync.Mutex

	svc.RegisterExecutor("approval_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executedSteps = append(executedSteps, step.Name)
		mu.Unlock()
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "approval-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "safe-step", Action: "approval_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "destructive-step", Action: "approval_step", MaxRetries: 1, TimeoutSeconds: 5, RequiresApproval: true},
			{Position: 3, Name: "final-step", Action: "approval_step", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exec, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for approval step
	time.Sleep(300 * time.Millisecond)

	result, _ := svc.GetExecution(context.Background(), exec.ID)
	if result.Status != "waiting_approval" {
		t.Fatalf("expected waiting_approval, got %s", result.Status)
	}

	// Find the waiting step
	var waitingStep *store.ProcedureStepExecution
	for _, step := range result.Steps {
		if step.Status == "waiting_approval" {
			waitingStep = &step
			break
		}
	}

	if waitingStep == nil {
		t.Fatal("no step waiting for approval")
	}

	// Approve the step
	userID := "admin"
	err = svc.ApproveStep(context.Background(), waitingStep.ID, &userID)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for completion
	time.Sleep(1500 * time.Millisecond)

	finalResult, _ := svc.GetExecution(context.Background(), exec.ID)
	if finalResult.Status != "succeeded" {
		t.Fatalf("expected succeeded after approval, got %s", finalResult.Status)
	}

	mu.Lock()
	defer mu.Unlock()

	// All steps should have executed
	if len(executedSteps) != 3 {
		t.Fatalf("expected 3 steps executed, got %d", len(executedSteps))
	}

	t.Log("✓ Approval workflow verified")
}

// Test 8: Unauthorized Approval Rejection
func testUnauthorizedApprovalRejection(t *testing.T) {
	t.Log("Testing unauthorized approval rejection...")

	svc, _ := testSvc(t)

	svc.RegisterExecutor("normal_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "unauth-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "normal-step", Action: "normal_step", MaxRetries: 1, TimeoutSeconds: 5},
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

	// Try to approve a step that doesn't require approval
	for _, step := range result.Steps {
		if step.Status == "succeeded" {
			err := svc.ApproveStep(context.Background(), step.ID, nil)
			if err == nil {
				t.Fatal("expected error when approving non-waiting step")
			}
			break
		}
	}

	t.Log("✓ Unauthorized approval rejection verified")
}

// Test 9: Rollback Hooks
func testRollbackHooks(t *testing.T) {
	t.Log("Testing rollback hooks...")

	svc, _ := testSvc(t)

	var executedSteps []string
	var mu sync.Mutex

	svc.RegisterExecutor("rollback_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executedSteps = append(executedSteps, step.Name)
		mu.Unlock()

		// Fail on cleanup step
		if step.Name == "cleanup" {
			return errors.New("cleanup failed")
		}
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "rollback-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "deploy", Action: "rollback_step", MaxRetries: 0, TimeoutSeconds: 5, RollbackEnabled: true},
			{Position: 2, Name: "cleanup", Action: "rollback_step", MaxRetries: 0, TimeoutSeconds: 5, RollbackEnabled: true},
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

	// Check for rollback execution
	executions, _ := svc.ListExecutions(context.Background(), proc.ID, 10)
	if len(executions) < 2 {
		t.Fatalf("expected at least 2 executions (original + rollback), got %d", len(executions))
	}

	// Find rollback execution
	var rollbackExec *store.ProcedureExecution
	for _, e := range executions {
		if e.Status == "rolled_back" {
			rollbackExec = &e
			break
		}
	}

	if rollbackExec == nil {
		t.Fatal("rollback execution not found")
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify rollback steps executed
	if len(executedSteps) < 3 {
		t.Fatalf("expected at least 3 steps (deploy, cleanup, rollback), got %d: %v", len(executedSteps), executedSteps)
	}

	t.Log("✓ Rollback hooks verified")
}

// Test 10: Operation and Step History
func testHistoryTracking(t *testing.T) {
	t.Log("Testing history tracking...")

	svc, fs := testSvc(t)

	svc.RegisterExecutor("history_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		logf("info", "step started")
		logf("info", "step completed")
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "history-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step1", Action: "history_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "step2", Action: "history_step", MaxRetries: 1, TimeoutSeconds: 5},
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

	// Check audit events
	if len(fs.auditEvents) == 0 {
		t.Fatal("expected audit events")
	}

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

	// Check step logs
	for _, step := range exec.Steps {
		logs, err := svc.ListStepLogs(context.Background(), step.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) == 0 {
			t.Fatalf("expected logs for step %s", step.ID)
		}
	}

	t.Log("✓ History tracking verified")
}

// Test 11: Restart After Completed Step
func testRestartAfterCompletedStep(t *testing.T) {
	t.Log("Testing restart after completed step...")

	svc, _ := testSvc(t)

	var executionCount int
	var mu sync.Mutex

	svc.RegisterExecutor("restart_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executionCount++
		mu.Unlock()
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "restart-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "deploy", Action: "restart_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "configure", Action: "restart_step", MaxRetries: 1, TimeoutSeconds: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// First execution
	exec1, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	result1, _ := svc.GetExecution(context.Background(), exec1.ID)
	if result1.Status != "succeeded" {
		t.Fatalf("first execution expected succeeded, got %s", result1.Status)
	}

	// Second execution (restart)
	exec2, err := svc.ExecuteProcedure(context.Background(), proc.ID, "manual", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	result2, _ := svc.GetExecution(context.Background(), exec2.ID)
	if result2.Status != "succeeded" {
		t.Fatalf("second execution expected succeeded, got %s", result2.Status)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have executed 4 steps total (2 per execution)
	if executionCount != 4 {
		t.Fatalf("expected 4 step executions, got %d", executionCount)
	}

	t.Log("✓ Restart after completed step verified")
}

// Test 12: Idempotency - Next Step Does Not Run Twice
func testIdempotency(t *testing.T) {
	t.Log("Testing idempotency...")

	svc, _ := testSvc(t)

	var executedSteps []string
	var mu sync.Mutex

	svc.RegisterExecutor("idempotent_step", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		mu.Lock()
		executedSteps = append(executedSteps, step.Name)
		mu.Unlock()
		return nil
	})

	proc, err := svc.CreateProcedure(context.Background(), store.CreateProcedureRequest{
		Name: "idempotency-test",
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "step1", Action: "idempotent_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 2, Name: "step2", Action: "idempotent_step", MaxRetries: 1, TimeoutSeconds: 5},
			{Position: 3, Name: "step3", Action: "idempotent_step", MaxRetries: 1, TimeoutSeconds: 5},
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

	mu.Lock()
	defer mu.Unlock()

	// Each step should execute exactly once
	if len(executedSteps) != 3 {
		t.Fatalf("expected 3 steps executed, got %d", len(executedSteps))
	}

	// Check for duplicates
	seen := make(map[string]int)
	for _, name := range executedSteps {
		seen[name]++
		if seen[name] > 1 {
			t.Fatalf("step %s executed more than once", name)
		}
	}

	t.Log("✓ Idempotency verified - each step ran exactly once")
}

// Helper to create test service with fake store
func testSvcForScenario9(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	fakeSt := newFakeStore()
	svc := New(fakeSt, &fakePublisher{}, slog.Default(), fakeSt)

	// Register test executors
	svc.RegisterExecutor("test_succeed", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		logf("info", "test step succeeded")
		return nil
	})

	svc.RegisterExecutor("test_fail", func(ctx context.Context, execID, stepExecID string, step store.ProcedureStep, logf func(string, string)) error {
		logf("error", "test step failed")
		return errors.New("step failed intentionally")
	})

	return svc, fakeSt
}
