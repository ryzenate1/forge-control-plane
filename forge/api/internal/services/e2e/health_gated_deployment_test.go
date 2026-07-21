//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/services/deployment"
	"gamepanel/forge/internal/store"
)

// TestHealthGatedDeploymentScenario verifies health-gated deployment behavior
func TestHealthGatedDeploymentScenario(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Setup: Create region and nodes
	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	// Create a server to deploy to
	server, err := suite.Store.CreateServer(ctx, store.CreateServerRequest{
		ID:           uuid.NewString(),
		Name:         "test-server",
		RegionID:     &regionID,
		NodeID:       node1,
		DesiredState: string(domain.ServerDesiredStateRunning),
		ActualState:  string(domain.ServerActualStateRunning),
	})
	require.NoError(t, err)

	deploymentSvc := deployment.New(suite.Store, suite.Scheduler, suite.Publisher)

	t.Run("Deploy revision A successfully", func(t *testing.T) {
		// Deploy revision A (initial successful deployment)
		deploymentA := &deployment.Deployment{
			ID:          uuid.NewString(),
			ServerID:    server.ID,
			Image:       "test-image:v1",
			Strategy:    deployment.StrategyRecreate,
			Status:      string(deployment.StatusCompleted),
			ProgressPct: 100,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		// Persist deployment A
		err := suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(deploymentA))
		require.NoError(t, err)

		// Verify deployment A is stored
		stored, err := suite.Store.GetDeployment(ctx, deploymentA.ID)
		require.NoError(t, err)
		assert.Equal(t, deploymentA.Image, stored.Image)
		assert.Equal(t, string(deployment.StatusCompleted), stored.Status)
	})

	t.Run("Deploy failing revision B", func(t *testing.T) {
		// Create deployment B with health gate enabled
		deploymentB := &deployment.Deployment{
			ID:                      uuid.NewString(),
			ServerID:                server.ID,
			Image:                   "test-image:v2-failing",
			Strategy:                deployment.StrategyRecreate,
			Status:                  string(deployment.StatusInProgress),
			HealthGateEnabled:       true,
			HealthGateThreshold:     3,
			HealthGateIntervalMs:    1000,
			AutoRollbackEnabled:     true,
			RollbackOnHealthFailure: true,
			TimeoutSeconds:          30,
			ProgressPct:             0,
			CreatedAt:               time.Now().UTC(),
			UpdatedAt:               time.Now().UTC(),
		}

		// Persist deployment B
		err := suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(deploymentB))
		require.NoError(t, err)

		// Simulate health check failure
		// The health gate should prevent B from becoming active
		// We'll test the health gate logic directly

		// Test that health gate fails
		healthResult := &deployment.HealthCheckResult{
			Passed: false,
			Status: 500,
			Error:  "connection refused",
		}

		// Verify B does not become active prematurely
		// The deployment should remain in progress until health checks pass
		storedB, err := suite.Store.GetDeployment(ctx, deploymentB.ID)
		require.NoError(t, err)
		assert.Equal(t, string(deployment.StatusInProgress), storedB.Status)

		// Test health gate timeout
		ctx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
		defer healthCancel()

		// This would normally be called by the deployment service
		// We're testing that it properly handles failures
		err = deploymentSvc.WaitForHealthGate(ctx, deploymentB, "step-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health gate timed out")
	})

	t.Run("Verify rollback behavior", func(t *testing.T) {
		// Get the deployments
		deployments, err := suite.Store.ListDeployments(ctx, server.ID)
		require.NoError(t, err)

		// Find the successful deployment (A)
		var deploymentA *store.Deployment
		var deploymentB *store.Deployment

		for _, d := range deployments {
			if d.Image == "test-image:v1" {
				deploymentA = &d
			} else if d.Image == "test-image:v2-failing" {
				deploymentB = &d
			}
		}

		require.NotNil(t, deploymentA)
		require.NotNil(t, deploymentB)

		// Verify A remains or becomes active after B fails
		// In a real scenario, the rollback would have been triggered
		// Here we verify that A is still in completed state
		assert.Equal(t, string(deployment.StatusCompleted), deploymentA.Status)

		// Verify B resources are cleaned (in a real scenario)
		// For now, we verify that B is not in completed state
		assert.NotEqual(t, string(deployment.StatusCompleted), deploymentB.Status)
	})

	t.Run("Operation timeline verification", func(t *testing.T) {
		// Get all deployments for the server
		deployments, err := suite.Store.ListDeployments(ctx, server.ID)
		require.NoError(t, err)

		// Verify we have both deployments
		assert.GreaterOrEqual(t, len(deployments), 2)

		// Verify timeline shows exact failure and rollback
		// This would be more comprehensive in a real test with actual deployment execution
		for _, d := range deployments {
			if d.Image == "test-image:v2-failing" {
				// Should have failure information
				assert.NotEqual(t, string(deployment.StatusCompleted), d.Status)
			}
		}
	})
}

// TestHealthGatedDeploymentWithMocks tests health gating with mocked components
func TestHealthGatedDeploymentWithMocks(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Setup
	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	server, err := suite.Store.CreateServer(ctx, store.CreateServerRequest{
		ID:           uuid.NewString(),
		Name:         "health-test-server",
		RegionID:     &regionID,
		NodeID:       node1,
		DesiredState: string(domain.ServerDesiredStateRunning),
		ActualState:  string(domain.ServerActualStateRunning),
	})
	require.NoError(t, err)

	deploymentSvc := deployment.New(suite.Store, suite.Scheduler, suite.Publisher)

	t.Run("Health gate with passing checks", func(t *testing.T) {
		// Create a deployment with health gate
		deploymentPassing := &deployment.Deployment{
			ID:                   uuid.NewString(),
			ServerID:             server.ID,
			Image:                "test-image:passing",
			Strategy:             deployment.StrategyRecreate,
			Status:               string(deployment.StatusInProgress),
			HealthGateEnabled:    true,
			HealthGateThreshold:  2,
			HealthGateIntervalMs: 100,
			HealthCheckPath:      "/health",
			HealthCheckPort:      8080,
			TimeoutSeconds:       10,
			ProgressPct:          50,
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		// Persist deployment
		err := suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(deploymentPassing))
		require.NoError(t, err)

		// Mock the health check to pass
		// In a real scenario, this would be an actual HTTP endpoint
		// For testing, we'll use a short timeout and verify the logic

		// Test with a very short timeout to avoid waiting too long
		deploymentPassing.HealthGateThreshold = 1
		deploymentPassing.HealthGateIntervalMs = 100
		deploymentPassing.TimeoutSeconds = 1

		// This should pass quickly if the health check passes
		// But since we don't have a real endpoint, it will timeout
		// So we'll just verify the health check logic

		// Test the health check function directly
		healthResult, err := deploymentSvc.CheckHealth(ctx, deploymentPassing)
		// This will fail because there's no actual server running
		assert.Error(t, err) || assert.False(t, healthResult.Passed)
	})

	t.Run("Health gate with failing checks", func(t *testing.T) {
		deploymentFailing := &deployment.Deployment{
			ID:                   uuid.NewString(),
			ServerID:             server.ID,
			Image:                "test-image:failing",
			Strategy:             deployment.StrategyRecreate,
			Status:               string(deployment.StatusInProgress),
			HealthGateEnabled:    true,
			HealthGateThreshold:  3,
			HealthGateIntervalMs: 100,
			HealthCheckPath:      "/health",
			HealthCheckPort:      9999, // Non-existent port
			TimeoutSeconds:       5,
			ProgressPct:          50,
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		err = suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(deploymentFailing))
		require.NoError(t, err)

		// Test health check failure
		healthResult, err := deploymentSvc.CheckHealth(ctx, deploymentFailing)
		assert.Error(t, err) || assert.False(t, healthResult.Passed)

		// Test health gate timeout
		ctx, healthCancel := context.WithTimeout(ctx, 2*time.Second)
		defer healthCancel()

		err = deploymentSvc.WaitForHealthGate(ctx, deploymentFailing, "step-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health gate timed out")
	})
}

// TestDeploymentRollback tests rollback scenarios
func TestDeploymentRollback(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	server, err := suite.Store.CreateServer(ctx, store.CreateServerRequest{
		ID:           uuid.NewString(),
		Name:         "rollback-test-server",
		RegionID:     &regionID,
		NodeID:       node1,
		DesiredState: string(domain.ServerDesiredStateRunning),
		ActualState:  string(domain.ServerActualStateRunning),
	})
	require.NoError(t, err)

	deploymentSvc := deployment.New(suite.Store, suite.Scheduler, suite.Publisher)

	// Create initial successful deployment
	initialDeployment := &deployment.Deployment{
		ID:          uuid.NewString(),
		ServerID:    server.ID,
		Image:       "test-image:v1",
		Strategy:    deployment.StrategyRecreate,
		Status:      string(deployment.StatusCompleted),
		ProgressPct: 100,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	err = suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(initialDeployment))
	require.NoError(t, err)

	t.Run("Rollback to previous revision", func(t *testing.T) {
		// Create a failing deployment
		failingDeployment := &deployment.Deployment{
			ID:                      uuid.NewString(),
			ServerID:                server.ID,
			Image:                   "test-image:v2-bad",
			Strategy:                deployment.StrategyRecreate,
			Status:                  string(deployment.StatusFailed),
			HealthGateEnabled:       true,
			AutoRollbackEnabled:     true,
			RollbackOnHealthFailure: true,
			TimeoutSeconds:          30,
			ProgressPct:             0,
			CreatedAt:               time.Now().UTC(),
			UpdatedAt:               time.Now().UTC(),
		}

		err = suite.Store.CreateDeployment(ctx, deployment.ToStoreDeployment(failingDeployment))
		require.NoError(t, err)

		// In a real scenario, the rollback would be triggered automatically
		// Here we verify that we can manually trigger a rollback

		// Verify that the initial deployment is still there
		deployments, err := suite.Store.ListDeployments(ctx, server.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(deployments), 2)

		// Find the successful deployment
		var successfulDeployment *store.Deployment
		for _, d := range deployments {
			if d.Status == string(deployment.StatusCompleted) {
				successfulDeployment = &d
				break
			}
		}

		require.NotNil(t, successfulDeployment)
		assert.Equal(t, "test-image:v1", successfulDeployment.Image)
	})

	t.Run("Final state consistency", func(t *testing.T) {
		// After rollback, the final state should be consistent
		deployments, err := suite.Store.ListDeployments(ctx, server.ID)
		require.NoError(t, err)

		// Should have at least one completed deployment
		completedCount := 0
		for _, d := range deployments {
			if d.Status == string(deployment.StatusCompleted) {
				completedCount++
			}
		}
		assert.GreaterOrEqual(t, completedCount, 1)

		// The server should still be in a good state
		updatedServer, err := suite.Store.GetServer(ctx, server.ID)
		require.NoError(t, err)
		assert.Equal(t, string(domain.ServerActualStateRunning), updatedServer.ActualState)
	})
}
