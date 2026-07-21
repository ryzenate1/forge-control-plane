package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TestReport holds the complete test report
type TestReport struct {
	mu        sync.Mutex
	Metadata  TestMetadata     `json:"metadata"`
	Scenarios []ScenarioReport `json:"scenarios"`
	Summary   TestSummary      `json:"summary"`
}

// TestMetadata holds metadata about the test run
type TestMetadata struct {
	Timestamp   time.Time `json:"timestamp"`
	Environment string    `json:"environment"`
	DatabaseURL string    `json:"databaseUrl"`
	GitCommit   string    `json:"gitCommit"`
	GitBranch   string    `json:"gitBranch"`
	Duration    string    `json:"duration"`
}

// ScenarioReport holds the report for a single scenario
type ScenarioReport struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Status      string           `json:"status"` // "passed", "failed", "skipped"
	Tests       []TestCaseReport `json:"tests"`
	StartTime   time.Time        `json:"startTime"`
	EndTime     time.Time        `json:"endTime"`
	Duration    string           `json:"duration"`
	Error       string           `json:"error,omitempty"`
}

// TestCaseReport holds the report for a single test case
type TestCaseReport struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // "passed", "failed", "skipped"
	Duration  string    `json:"duration"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// TestSummary holds summary statistics
type TestSummary struct {
	TotalScenarios   int       `json:"totalScenarios"`
	PassedScenarios  int       `json:"passedScenarios"`
	FailedScenarios  int       `json:"failedScenarios"`
	SkippedScenarios int       `json:"skippedScenarios"`
	TotalTests       int       `json:"totalTests"`
	PassedTests      int       `json:"passedTests"`
	FailedTests      int       `json:"failedTests"`
	SkippedTests     int       `json:"skippedTests"`
	StartTime        time.Time `json:"startTime"`
	EndTime          time.Time `json:"endTime"`
	TotalDuration    string    `json:"totalDuration"`
	SuccessRate      float64   `json:"successRate"`
}

// Global report instance
var globalReport = &TestReport{
	Metadata: TestMetadata{
		Timestamp: time.Now().UTC(),
	},
	Scenarios: make([]ScenarioReport, 0),
}

// InitializeReport initializes the global test report
func InitializeReport(env, dbURL, gitCommit, gitBranch string) {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	globalReport.Metadata.Environment = env
	globalReport.Metadata.DatabaseURL = dbURL
	globalReport.Metadata.GitCommit = gitCommit
	globalReport.Metadata.GitBranch = gitBranch
	globalReport.Summary.StartTime = time.Now().UTC()
}

// AddScenario adds a scenario to the report
func AddScenario(name, description string) *ScenarioReport {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	scenario := ScenarioReport{
		Name:        name,
		Description: description,
		Status:      "passed",
		Tests:       make([]TestCaseReport, 0),
		StartTime:   time.Now().UTC(),
	}
	globalReport.Scenarios = append(globalReport.Scenarios, scenario)
	return &globalReport.Scenarios[len(globalReport.Scenarios)-1]
}

// AddTestCase adds a test case to a scenario
func AddTestCase(scenarioName, testName, status, errorMsg, duration string) {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	for i := range globalReport.Scenarios {
		if globalReport.Scenarios[i].Name == scenarioName {
			globalReport.Scenarios[i].Tests = append(globalReport.Scenarios[i].Tests, TestCaseReport{
				Name:      testName,
				Status:    status,
				Duration:  duration,
				Error:     errorMsg,
				Timestamp: time.Now().UTC(),
			})

			// Update scenario status
			if status == "failed" && globalReport.Scenarios[i].Status == "passed" {
				globalReport.Scenarios[i].Status = "failed"
				globalReport.Scenarios[i].Error = errorMsg
			}
			return
		}
	}
}

// EndScenario marks the end of a scenario
func EndScenario(scenarioName string) {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	for i := range globalReport.Scenarios {
		if globalReport.Scenarios[i].Name == scenarioName {
			globalReport.Scenarios[i].EndTime = time.Now().UTC()
			globalReport.Scenarios[i].Duration = globalReport.Scenarios[i].EndTime.Sub(globalReport.Scenarios[i].StartTime).String()
			return
		}
	}
}

// FinalizeReport finalizes the report and calculates summary
func FinalizeReport() {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	globalReport.Summary.EndTime = time.Now().UTC()
	globalReport.Summary.TotalDuration = globalReport.Summary.EndTime.Sub(globalReport.Summary.StartTime).String()

	for _, scenario := range globalReport.Scenarios {
		globalReport.Summary.TotalScenarios++

		switch scenario.Status {
		case "passed":
			globalReport.Summary.PassedScenarios++
		case "failed":
			globalReport.Summary.FailedScenarios++
		case "skipped":
			globalReport.Summary.SkippedScenarios++
		}

		for _, test := range scenario.Tests {
			globalReport.Summary.TotalTests++

			switch test.Status {
			case "passed":
				globalReport.Summary.PassedTests++
			case "failed":
				globalReport.Summary.FailedTests++
			case "skipped":
				globalReport.Summary.SkippedTests++
			}
		}
	}

	// Calculate success rate
	if globalReport.Summary.TotalTests > 0 {
		globalReport.Summary.SuccessRate = float64(globalReport.Summary.PassedTests) / float64(globalReport.Summary.TotalTests) * 100
	}
}

// SaveReport saves the report to a JSON file
func SaveReport(filename string) error {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(globalReport, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// SaveMarkdownReport saves the report in Markdown format
func SaveMarkdownReport(filename string) error {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write markdown report
	fmt.Fprintf(file, "# E2E Test Report\n\n")

	// Metadata
	fmt.Fprintf(file, "## Test Metadata\n\n")
	fmt.Fprintf(file, "- **Timestamp**: %s\n", globalReport.Metadata.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(file, "- **Environment**: %s\n", globalReport.Metadata.Environment)
	fmt.Fprintf(file, "- **Database URL**: %s\n", globalReport.Metadata.DatabaseURL)
	fmt.Fprintf(file, "- **Git Commit**: %s\n", globalReport.Metadata.GitCommit)
	fmt.Fprintf(file, "- **Git Branch**: %s\n", globalReport.Metadata.GitBranch)
	fmt.Fprintf(file, "- **Total Duration**: %s\n\n", globalReport.Summary.TotalDuration)

	// Summary
	fmt.Fprintf(file, "## Test Summary\n\n")
	fmt.Fprintf(file, "| Metric | Value |\n")
	fmt.Fprintf(file, "|--------|-------|\n")
	fmt.Fprintf(file, "| Total Scenarios | %d |\n", globalReport.Summary.TotalScenarios)
	fmt.Fprintf(file, "| Passed Scenarios | %d |\n", globalReport.Summary.PassedScenarios)
	fmt.Fprintf(file, "| Failed Scenarios | %d |\n", globalReport.Summary.FailedScenarios)
	fmt.Fprintf(file, "| Skipped Scenarios | %d |\n", globalReport.Summary.SkippedScenarios)
	fmt.Fprintf(file, "| Total Tests | %d |\n", globalReport.Summary.TotalTests)
	fmt.Fprintf(file, "| Passed Tests | %d |\n", globalReport.Summary.PassedTests)
	fmt.Fprintf(file, "| Failed Tests | %d |\n", globalReport.Summary.FailedTests)
	fmt.Fprintf(file, "| Skipped Tests | %d |\n", globalReport.Summary.SkippedTests)
	fmt.Fprintf(file, "| Success Rate | %.2f%% |\n\n", globalReport.Summary.SuccessRate)

	// Scenario Details
	fmt.Fprintf(file, "## Scenario Details\n\n")

	for _, scenario := range globalReport.Scenarios {
		fmt.Fprintf(file, "### %s\n", scenario.Name)
		fmt.Fprintf(file, "**Description**: %s\n", scenario.Description)
		fmt.Fprintf(file, "**Status**: %s\n", scenario.Status)
		fmt.Fprintf(file, "**Duration**: %s\n", scenario.Duration)
		if scenario.Error != "" {
			fmt.Fprintf(file, "**Error**: %s\n", scenario.Error)
		}
		fmt.Fprintf(file, "\n")

		fmt.Fprintf(file, "| Test Case | Status | Duration | Error |\n")
		fmt.Fprintf(file, "|-----------|--------|----------|-------|\n")

		for _, test := range scenario.Tests {
			error := test.Error
			if error == "" {
				error = "-"
			}
			fmt.Fprintf(file, "| %s | %s | %s | %s |\n", test.Name, test.Status, test.Duration, error)
		}

		fmt.Fprintf(file, "\n")
	}

	// Defects and Recommendations
	fmt.Fprintf(file, "## Defects Found\n\n")
	fmt.Fprintf(file, "*No defects recorded in this test run.*\n\n")

	fmt.Fprintf(file, "## Recommendations\n\n")
	fmt.Fprintf(file, "*No recommendations from this test run.*\n")

	return nil
}

// PrintReport prints the report to stdout
func PrintReport() {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	fmt.Println("\n========================================")
	fmt.Println("E2E Test Report")
	fmt.Println("========================================")
	fmt.Printf("Timestamp: %s\n", globalReport.Metadata.Timestamp.Format(time.RFC3339))
	fmt.Printf("Environment: %s\n", globalReport.Metadata.Environment)
	fmt.Printf("Database URL: %s\n", globalReport.Metadata.DatabaseURL)
	fmt.Printf("Git Commit: %s\n", globalReport.Metadata.GitCommit)
	fmt.Printf("Git Branch: %s\n", globalReport.Metadata.GitBranch)
	fmt.Printf("Total Duration: %s\n\n", globalReport.Summary.TotalDuration)

	fmt.Println("Summary:")
	fmt.Printf("  Total Scenarios: %d\n", globalReport.Summary.TotalScenarios)
	fmt.Printf("  Passed: %d, Failed: %d, Skipped: %d\n",
		globalReport.Summary.PassedScenarios,
		globalReport.Summary.FailedScenarios,
		globalReport.Summary.SkippedScenarios)
	fmt.Printf("  Total Tests: %d\n", globalReport.Summary.TotalTests)
	fmt.Printf("  Passed: %d, Failed: %d, Skipped: %d\n",
		globalReport.Summary.PassedTests,
		globalReport.Summary.FailedTests,
		globalReport.Summary.SkippedTests)
	fmt.Printf("  Success Rate: %.2f%%\n\n", globalReport.Summary.SuccessRate)

	fmt.Println("Scenario Results:")
	for _, scenario := range globalReport.Scenarios {
		statusIcon := "✓"
		if scenario.Status == "failed" {
			statusIcon = "✗"
		} else if scenario.Status == "skipped" {
			statusIcon = "⊘"
		}
		fmt.Printf("  %s %s (%s) - %s\n", statusIcon, scenario.Name, scenario.Status, scenario.Duration)
		if scenario.Error != "" {
			fmt.Printf("    Error: %s\n", scenario.Error)
		}
	}

	fmt.Println("\n========================================")
}

// GetReport returns the global report
func GetReport() *TestReport {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	// Create a copy to avoid race conditions
	report := &TestReport{
		Metadata:  globalReport.Metadata,
		Scenarios: make([]ScenarioReport, len(globalReport.Scenarios)),
		Summary:   globalReport.Summary,
	}

	for i, scenario := range globalReport.Scenarios {
		report.Scenarios[i] = ScenarioReport{
			Name:        scenario.Name,
			Description: scenario.Description,
			Status:      scenario.Status,
			Tests:       make([]TestCaseReport, len(scenario.Tests)),
			StartTime:   scenario.StartTime,
			EndTime:     scenario.EndTime,
			Duration:    scenario.Duration,
			Error:       scenario.Error,
		}
		for j, test := range scenario.Tests {
			report.Scenarios[i].Tests[j] = TestCaseReport{
				Name:      test.Name,
				Status:    test.Status,
				Duration:  test.Duration,
				Error:     test.Error,
				Timestamp: test.Timestamp,
			}
		}
	}

	return report
}

// ResetReport resets the global report
func ResetReport() {
	globalReport.mu.Lock()
	defer globalReport.mu.Unlock()

	globalReport = &TestReport{
		Metadata: TestMetadata{
			Timestamp: time.Now().UTC(),
		},
		Scenarios: make([]ScenarioReport, 0),
	}
}
