//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMain is the entry point for all E2E tests
func TestMain(m *testing.M) {
	// Initialize test reporting
	initReporting()

	// Run all tests
	os.Exit(m.Run())
}

// initReporting initializes the test reporting infrastructure
func initReporting() {
	// Get environment information
	env := getEnv("E2E_ENVIRONMENT", "local")
	dbURL := getEnv("TEST_DATABASE_URL", "")
	gitCommit := getGitCommit()
	gitBranch := getGitBranch()

	// Initialize the report
	InitializeReport(env, dbURL, gitCommit, gitBranch)

	// Set up cleanup on exit
	// go test doesn't provide a clean way to do this, so we'll rely on the OS
}

// getEnv gets an environment variable with a default
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

// getGitCommit gets the current git commit
func getGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// getGitBranch gets the current git branch
func getGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	branch := strings.TrimSpace(string(output))
	// Remove "refs/heads/" prefix
	return strings.TrimPrefix(branch, "refs/heads/")
}

// TestAllScenarios runs all E2E scenarios and generates a comprehensive report
func TestAllScenarios(t *testing.T) {
	// Start timer for the entire test suite
	startTime := time.Now()

	// Run each scenario as a subtest
	scenarios := []struct {
		name        string
		description string
		fn          func(*testing.T)
	}{
		{
			name:        "Replicated Application",
			description: "Tests replicated application lifecycle including scaling, placement, and cleanup",
			fn:          TestReplicatedApplicationScenario,
		},
		{
			name:        "Health-Gated Deployment",
			description: "Tests health-gated deployment with rollback and failure handling",
			fn:          TestHealthGatedDeploymentScenario,
		},
		{
			name:        "Node Failure",
			description: "Tests node failure detection, recovery, and instance replacement",
			fn:          TestNodeFailureScenario,
		},
		{
			name:        "GitOps Compose Stack",
			description: "Tests Git-backed Compose stack with webhooks, drift detection, and reconciliation",
			fn:          TestGitOpsComposeStackScenario,
		},
		{
			name:        "Remote Build and Registry",
			description: "Tests remote build capabilities, registry integration, and image management",
			fn:          TestRemoteBuildAndRegistryScenario,
		},
	}

	// Run each scenario
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Add scenario to report
			scenarioReport := AddScenario(scenario.name, scenario.description)

			// Run the scenario
			start := time.Now()

			// Run in a goroutine to capture panics
			panicked := false
			var panicErr interface{}

			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
						panicErr = r
					}
				}()

				scenario.fn(t)
			}()

			duration := time.Since(start)

			// End the scenario
			EndScenario(scenario.name)

			// Update scenario status based on test result
			if panicked {
				scenarioReport.Status = "failed"
				scenarioReport.Error = fmt.Sprintf("Panic: %v", panicErr)
				t.Error(fmt.Sprintf("Scenario panicked: %v", panicErr))
			} else if t.Failed() {
				scenarioReport.Status = "failed"
				scenarioReport.Error = "One or more tests failed"
			} else {
				scenarioReport.Status = "passed"
			}

			// Add a test case for the scenario itself
			AddTestCase(scenario.name, scenario.name, scenarioReport.Status, scenarioReport.Error, duration.String())
		})
	}

	// Finalize the report
	FinalizeReport()

	// Print the report
	PrintReport()

	// Save reports to files
	if err := SaveReport("test-reports/e2e-report.json"); err != nil {
		t.Logf("Failed to save JSON report: %v", err)
	}

	if err := SaveMarkdownReport("test-reports/e2e-report.md"); err != nil {
		t.Logf("Failed to save Markdown report: %v", err)
	}

	// Log total duration
	totalDuration := time.Since(startTime)
	t.Logf("Total E2E test duration: %s", totalDuration)
}

// TestIndividualScenarios allows running individual scenarios
func TestIndividualScenarios(t *testing.T) {
	// This test can be used to run specific scenarios individually
	// Use: go test -v -tags=e2e -run TestIndividualScenarios/Replicated_Application

	t.Run("Replicated Application", TestReplicatedApplicationScenario)
	t.Run("Health-Gated Deployment", TestHealthGatedDeploymentScenario)
	t.Run("Node Failure", TestNodeFailureScenario)
	t.Run("GitOps Compose Stack", TestGitOpsComposeStackScenario)
	t.Run("Remote Build and Registry", TestRemoteBuildAndRegistryScenario)
}

// TestEdgeCases runs all edge case tests
func TestEdgeCases(t *testing.T) {
	t.Run("Replicated Application Edge Cases", TestReplicatedApplicationEdgeCases)
	t.Run("Health Gated Deployment Edge Cases", TestHealthGatedDeploymentWithMocks)
	t.Run("Node Heartbeat Expiry", TestNodeHeartbeatExpiry)
	t.Run("Node Evacuation", TestNodeEvacuation)
	t.Run("GitOps Compose Stack Edge Cases", TestGitOpsComposeStackEdgeCases)
	t.Run("Remote Build Edge Cases", TestRemoteBuildEdgeCases)
}

// TestWithContextTimeout runs tests with a specific timeout
func TestWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run tests with the context
	t.Run("Replicated Application with Timeout", func(t *testing.T) {
		TestReplicatedApplicationScenario(t)
	})
}

// TestDatabaseConnectivity verifies database connectivity before running other tests
func TestDatabaseConnectivity(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		t.Skip("Skipping database connectivity test - no database configured")
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test basic database operations
	err := suite.Store.Health(ctx)
	if err != nil {
		t.Fatalf("Database health check failed: %v", err)
	}

	t.Log("Database connectivity verified")
}
