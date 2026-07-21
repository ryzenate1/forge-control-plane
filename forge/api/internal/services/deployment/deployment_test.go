package deployment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteDeployment_SuccessfulRollout_Recreate(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-exec", StrategyRecreate)

	err := svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	updated, err := svc.GetDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, updated.Status)
	assert.Equal(t, 100, updated.ProgressPct)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, steps)

	for _, step := range steps {
		assert.Equal(t, StepStatusCompleted, step.Status, "step %s should be completed", step.StepName)
	}
}

func TestExecuteDeployment_SuccessfulRollout_Rolling(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-exec-rolling", StrategyRolling)
	dep.HealthGateEnabled = false

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusCompleted, updated.Status)
}

func TestExecuteDeployment_SuccessfulRollout_BlueGreen(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-exec-bg", StrategyBlueGreen)
	dep.HealthGateEnabled = false

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusCompleted, updated.Status)

	revisions, err := svc.ListRevisions(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Len(t, revisions, 1)
	assert.Equal(t, 1, revisions[0].RevisionNumber)
}

func TestExecuteDeployment_HealthGateConfigured(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-hc", StrategyRecreate)
	dep.HealthGateEnabled = true
	dep.HealthCheckPath = "/health"
	dep.HealthCheckPort = 65535
	dep.HealthGateThreshold = 1
	dep.HealthGateIntervalMs = 100
	dep.TimeoutSeconds = 5

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health gate timed out")

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusFailed, updated.Status)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	var healthStep *DeploymentStep
	for _, s := range steps {
		if s.StepName == StepHealthGate {
			healthStep = s
			break
		}
	}
	require.NotNil(t, healthStep)
	assert.Equal(t, StepStatusFailed, healthStep.Status)
}

func TestExecuteDeployment_HealthGateSkippedWhenDisabled(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-hc-skip", StrategyRecreate)
	dep.HealthGateEnabled = false
	dep.HealthCheckPath = ""
	dep.HealthCheckPort = 0

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusCompleted, updated.Status)
}

func TestExecuteDeployment_Cancellation(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-cancel", StrategyRecreate)

	cancelled, err := svc.CancelDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, cancelled.Status)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in pending state")
}

func TestExecuteDeployment_DuplicateRequest(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-dup", StrategyRecreate)

	err := svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.Error(t, err)
}

func TestManualRollback(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-mr", StrategyBlueGreen)
	dep.Status = StatusCompleted

	rev, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		Description: "v1",
	})
	require.NoError(t, err)

	rev2, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26@sha256:def456abc123def456abc123def456abc123def456abc123def456abc123def456",
		Description: "v2",
	})
	require.NoError(t, err)

	err = svc.store.UpdateDeploymentCurrentRevision(context.Background(), dep.ID, &rev2.ID)
	require.NoError(t, err)

	result, err := svc.RollbackToRevision(context.Background(), dep.ID, rev.ID)
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd", result.Image)
	assert.Equal(t, StatusInProgress, result.Status)
}

func TestAutoRollbackOnHealthFailure(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-auto-rb", StrategyRecreate)
	dep.HealthGateEnabled = true
	dep.HealthCheckPath = "/health"
	dep.HealthCheckPort = 65535
	dep.HealthGateThreshold = 1
	dep.HealthGateIntervalMs = 100
	dep.TimeoutSeconds = 5
	dep.AutoRollbackEnabled = true
	dep.RollbackOnHealthFailure = true

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	rev1, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		Description: "v1",
	})
	require.NoError(t, err)
	err = svc.store.UpdateDeploymentCurrentRevision(context.Background(), dep.ID, &rev1.ID)
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.Error(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusRolledBack, updated.Status)
}

func TestRollbackToPreviousRevision(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-rprev", StrategyRecreate)

	_, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		Description: "v1",
	})
	require.NoError(t, err)

	rev2, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26@sha256:def456abc123def456abc123def456abc123def456abc123def456abc123def456",
		Description: "v2",
	})
	require.NoError(t, err)

	err = svc.store.UpdateDeploymentCurrentRevision(context.Background(), dep.ID, &rev2.ID)
	require.NoError(t, err)

	result, err := svc.RollbackToPrevious(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd", result.Image)
}

func TestRollbackFailureNoRevisions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-rf", StrategyRecreate)

	_, err := svc.RollbackToPrevious(context.Background(), dep.ID)
	assert.ErrorIs(t, err, ErrNoRevisions)
}

func TestStepCreation(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-steps", StrategyBlueGreen)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Empty(t, steps)

	err = svc.createSteps(context.Background(), dep.ID, StrategyBlueGreen, true)
	require.NoError(t, err)

	steps, err = svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 7)

	expectedSteps := []string{StepInit, StepProvision, StepHealthGate, StepPromote, StepVerify, StepDrainOld, StepComplete}
	for i, s := range steps {
		assert.Equal(t, expectedSteps[i], s.StepName)
		assert.Equal(t, StepStatusPending, s.Status)
		assert.Equal(t, i, s.StepNumber)
	}
}

func TestStepCreationWithoutHealthGate(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-steps-nohg", StrategyBlueGreen)

	err := svc.createSteps(context.Background(), dep.ID, StrategyBlueGreen, false)
	require.NoError(t, err)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 5)

	for _, s := range steps {
		assert.NotEqual(t, StepHealthGate, s.StepName)
	}
}

func TestHealthGateBeforePromotion_BlueGreen(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-hg-order", StrategyBlueGreen)
	dep.HealthGateEnabled = true

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.createSteps(context.Background(), dep.ID, StrategyBlueGreen, true)
	require.NoError(t, err)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 7)

	expectedSteps := []string{StepInit, StepProvision, StepHealthGate, StepPromote, StepVerify, StepDrainOld, StepComplete}
	for i, s := range steps {
		assert.Equal(t, expectedSteps[i], s.StepName, "step %d should be %s", i, expectedSteps[i])
	}

	hgIdx := -1
	promoteIdx := -1
	for i, s := range steps {
		if s.StepName == StepHealthGate {
			hgIdx = i
		}
		if s.StepName == StepPromote {
			promoteIdx = i
		}
	}
	assert.True(t, hgIdx >= 0 && promoteIdx >= 0 && hgIdx < promoteIdx, "health_gate must come before promote")
}

func TestHealthGateBeforeDrainCanary_Canary(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-hg-canary", StrategyCanary)
	dep.HealthGateEnabled = true

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.createSteps(context.Background(), dep.ID, StrategyCanary, true)
	require.NoError(t, err)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 6)

	expectedSteps := []string{StepInit, StepProvision, StepHealthGate, StepDrainCanary, StepPromote, StepComplete}
	for i, s := range steps {
		assert.Equal(t, expectedSteps[i], s.StepName, "step %d should be %s", i, expectedSteps[i])
	}

	hgIdx := -1
	drainIdx := -1
	for i, s := range steps {
		if s.StepName == StepHealthGate {
			hgIdx = i
		}
		if s.StepName == StepDrainCanary {
			drainIdx = i
		}
	}
	assert.True(t, hgIdx >= 0 && drainIdx >= 0 && hgIdx < drainIdx, "health_gate must come before drain_canary")
}

func TestCleanupFailedDeployment(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-clean", StrategyRecreate)
	dep.Status = StatusFailed

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.CleanupDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	cleaned, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusCancelled, cleaned.Status)
}

func TestCancelCompletedDeploymentFails(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-ccf", StrategyRecreate)
	dep.Status = StatusCompleted

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	_, err = svc.CancelDeployment(context.Background(), dep.ID)
	require.Error(t, err)
}

func TestResumeDeployments(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-resume", StrategyRecreate)
	dep.HealthGateEnabled = false
	dep.TimeoutAt = nil

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ResumeDeployments(context.Background())
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusCompleted, updated.Status)
}

func TestResumeDeploymentsTimeoutExpired(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	now := time.Now().UTC()
	dep := createTestPendingDeployment(t, svc, "test-server-resume-to", StrategyRecreate)
	past := now.Add(-1 * time.Hour)
	dep.TimeoutAt = &past

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ResumeDeployments(context.Background())
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, StatusFailed, updated.Status)
	assert.Contains(t, updated.Error, "timed out")
}

func TestDeploymentStepsProgressPct(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-progress", StrategyRecreate)

	err := svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, 100, updated.ProgressPct)
}

func TestDuplicateDeploymentRequest(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-ddr", StrategyRecreate)

	err := svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.Error(t, err)
}

func TestStepLifecycle(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-step-lc", StrategyRecreate)

	err := svc.createSteps(context.Background(), dep.ID, StrategyRecreate, false)
	require.NoError(t, err)

	steps, err := svc.ListSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 3)

	assert.Equal(t, StepStatusPending, steps[0].Status)
	assert.Equal(t, StepInit, steps[0].StepName)
	assert.Equal(t, StepProvision, steps[1].StepName)
	assert.Equal(t, StepComplete, steps[2].StepName)

	err = svc.markStepStarted(context.Background(), steps[0].ID)
	require.NoError(t, err)

	steps, _ = svc.ListSteps(context.Background(), dep.ID)
	assert.Equal(t, StepStatusRunning, steps[0].Status)
	assert.NotNil(t, steps[0].StartedAt)

	err = svc.markStepCompleted(context.Background(), steps[0].ID)
	require.NoError(t, err)

	steps, _ = svc.ListSteps(context.Background(), dep.ID)
	assert.Equal(t, StepStatusCompleted, steps[0].Status)
	assert.NotNil(t, steps[0].CompletedAt)
}

func TestGetStep(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-get-step", StrategyRecreate)

	err := svc.createSteps(context.Background(), dep.ID, StrategyRecreate, false)
	require.NoError(t, err)

	steps, _ := svc.ListSteps(context.Background(), dep.ID)
	require.NotEmpty(t, steps)

	step, err := svc.GetStep(context.Background(), steps[0].ID)
	require.NoError(t, err)
	assert.Equal(t, steps[0].ID, step.ID)
	assert.Equal(t, steps[0].StepName, step.StepName)
}

func TestGetStepNotFound(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	_, err := svc.GetStep(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrStepNotFound)
}

func TestCreateRevisionSnapshotsAtDeployTime(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-snap", StrategyRecreate)
	dep.Image = "nginx:1.27@sha256:789abc123def456abc123def456abc123def456abc123def456abc123def456abc"

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	err = svc.ExecuteDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	revisions, err := svc.ListRevisions(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	assert.Equal(t, "nginx:1.27@sha256:789abc123def456abc123def456abc123def456abc123def456abc123def456abc", revisions[0].ImageRef)
	assert.NotEmpty(t, revisions[0].ConfigHash)
}

func TestStrategyStepDefinitions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	strategies := []struct {
		strategy Strategy
		steps    []string
	}{
		{StrategyRecreate, []string{StepInit, StepProvision, StepComplete}},
		{StrategyRolling, []string{StepInit, StepScaleUp, StepScaleDown, StepComplete}},
		{StrategyBlueGreen, []string{StepInit, StepProvision, StepPromote, StepDrainOld, StepComplete}},
		{StrategyCanary, []string{StepInit, StepProvision, StepDrainCanary, StepPromote, StepComplete}},
	}

	for _, tc := range strategies {
		t.Run(string(tc.strategy), func(t *testing.T) {
			dep := createTestPendingDeployment(t, svc, "test-server-"+string(tc.strategy), tc.strategy)
			dep.HealthGateEnabled = false

			err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
			require.NoError(t, err)

			err = svc.ExecuteDeployment(context.Background(), dep.ID)
			require.NoError(t, err)

			steps, _ := svc.ListSteps(context.Background(), dep.ID)
			require.Len(t, steps, len(tc.steps))
			for i, s := range steps {
				assert.Equal(t, tc.steps[i], s.StepName)
				assert.Equal(t, StepStatusCompleted, s.Status)
			}
		})
	}
}

func TestHealthCheckResult(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := &Deployment{
		HealthCheckPath: "",
		HealthCheckPort: 0,
	}
	result, err := svc.CheckHealth(context.Background(), dep)
	require.NoError(t, err)
	assert.True(t, result.Passed)
}

func TestEnsureStoreMethods(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-store", StrategyRecreate)

	err := svc.createSteps(context.Background(), dep.ID, StrategyRecreate, false)
	require.NoError(t, err)

	steps, err := svc.store.ListDeploymentSteps(context.Background(), dep.ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)

	step, err := svc.store.GetDeploymentStep(context.Background(), steps[0].ID)
	require.NoError(t, err)
	assert.Equal(t, steps[0].ID, step.ID)
}

func TestUpdateDeploymentProgress(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-up", StrategyRecreate)

	err := svc.updateProgress(context.Background(), dep.ID, 50, 2, nil)
	require.NoError(t, err)

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	assert.Equal(t, 50, updated.ProgressPct)
	assert.Equal(t, 2, updated.NextStep)
}

func TestDeploymentHelper_checkHealth(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := &Deployment{
		HealthCheckPath:      "/nonexistent",
		HealthCheckPort:      1,
		TimeoutSeconds:       3,
		HealthGateEnabled:    true,
		HealthGateThreshold:  1,
		HealthGateIntervalMs: 100,
	}
	err := svc.WaitForHealthGate(context.Background(), dep, "dummy-step-id")
	require.Error(t, err)
}

func TestCleanupFailedDeploymentsOlderThan(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	now := time.Now().UTC()
	srvID := uuid.NewString()
	dep := &Deployment{
		ID:               uuid.NewString(),
		ServerID:         srvID,
		Strategy:         StrategyRecreate,
		Status:           StatusFailed,
		Image:            "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		BlueTargetID:     srvID + "-blue",
		CleanupOnFailure: true,
		CreatedAt:        oldTime,
		UpdatedAt:        now,
	}

	err := svc.store.CreateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	count, err := svc.CleanupFailedDeployments(context.Background(), 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestRollbackDeployment(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-rbd", StrategyBlueGreen)
	dep.ActiveTarget = "green"

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	result, err := svc.Rollback(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "blue", result.ActiveTarget)
}

func TestRollbackNoTarget(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-rbnt", StrategyBlueGreen)
	dep.ActiveTarget = "blue"

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	_, err = svc.Rollback(context.Background(), dep.ID)
	assert.ErrorIs(t, err, ErrNoRollback)
}

func TestDeploymentStepStatusTransitions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-st", StrategyRecreate)

	err := svc.createSteps(context.Background(), dep.ID, StrategyRecreate, false)
	require.NoError(t, err)

	steps, _ := svc.ListSteps(context.Background(), dep.ID)
	require.Len(t, steps, 3)

	_ = svc.markStepStarted(context.Background(), steps[0].ID)
	_ = svc.markStepFailed(context.Background(), steps[0].ID, "test error")

	steps, _ = svc.ListSteps(context.Background(), dep.ID)
	assert.Equal(t, StepStatusFailed, steps[0].Status)
	assert.Equal(t, "test error", steps[0].Error)

	_ = svc.markStepSkipped(context.Background(), steps[1].ID)
	steps, _ = svc.ListSteps(context.Background(), dep.ID)
	assert.Equal(t, StepStatusSkipped, steps[1].Status)
}

func TestListInProgressDeployments(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-lip", StrategyRecreate)
	dep.Status = StatusInProgress

	err := svc.store.UpdateDeployment(context.Background(), toStoreDeployment(dep))
	require.NoError(t, err)

	pending, err := svc.store.ListInProgressDeployments(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, pending)
}
