package cronjob

import (
	"context"
	"log/slog"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestNewCronJobService(t *testing.T) {
	svc, _ := New(nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.cron == nil {
		t.Error("expected cron instance to be initialized")
	}

	svc2, _ := New(nil, slog.Default())
	if svc2 == nil {
		t.Fatal("expected non-nil service with logger")
	}
}

func TestGenerateExecutionID(t *testing.T) {
	svc, _ := New(nil, nil)
	id1 := svc.GenerateExecutionID()
	id2 := svc.GenerateExecutionID()
	if id1 == "" {
		t.Error("expected non-empty execution ID")
	}
	if id1 == id2 {
		t.Error("expected unique execution IDs")
	}
}

func TestStopWithoutStart(t *testing.T) {
	svc, _ := New(nil, nil)
	svc.Stop()
}

func TestNextRunWithNoEntries(t *testing.T) {
	svc, _ := New(nil, nil)
	job := store.CronJob{ID: "nonexistent", Schedule: "*/5 * * * * *"}
	next := svc.NextRun(job)
	if next != nil {
		t.Error("expected nil for unscheduled job")
	}
}

func TestNextRunAfterScheduling(t *testing.T) {
	svc, _ := New(nil, nil)
	svc.cron.Start()
	defer svc.Stop()

	svc.mu.Lock()
	entryID, _ := svc.cron.AddFunc("*/1 * * * * *", func() {})
	svc.entries["test-next-run"] = entryID
	svc.mu.Unlock()

	job := store.CronJob{ID: "test-next-run", Schedule: "*/1 * * * * *"}
	next := svc.NextRun(job)
	if next == nil {
		t.Error("expected non-nil next run time for scheduled cron entry")
	}
}

func TestRescheduleJobDisables(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(nil, nil)
	svc.cron.Start()
	defer svc.Stop()

	job := store.CronJob{
		ID:       "disable-test",
		Schedule: "*/5 * * * * *",
		Command:  "echo hello",
		Type:     "shell",
		Enabled:  false,
	}

	err := svc.RescheduleJob(ctx, job)
	if err != nil {
		t.Fatalf("RescheduleJob (disabled): %v", err)
	}

	svc.mu.Lock()
	_, exists := svc.entries["disable-test"]
	svc.mu.Unlock()
	if exists {
		t.Error("expected disabled job entry to be removed")
	}
}

func TestRescheduleJobEnabled(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(nil, nil)
	svc.cron.Start()
	defer svc.Stop()

	job := store.CronJob{
		ID:       "enable-test",
		Schedule: "*/5 * * * * *",
		Command:  "echo hello",
		Type:     "shell",
		Enabled:  true,
	}

	err := svc.RescheduleJob(ctx, job)
	if err != nil {
		t.Fatalf("RescheduleJob (enabled): %v", err)
	}

	next := svc.NextRun(job)
	if next == nil {
		t.Error("expected non-nil next run for enabled job")
	}
}

func TestRescheduleJobReschedules(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(nil, nil)
	svc.cron.Start()
	defer svc.Stop()

	job := store.CronJob{
		ID:       "reschedule-test",
		Schedule: "*/5 * * * * *",
		Command:  "echo hello",
		Type:     "shell",
		Enabled:  true,
	}

	if err := svc.RescheduleJob(ctx, job); err != nil {
		t.Fatalf("first RescheduleJob: %v", err)
	}

	if err := svc.RescheduleJob(ctx, job); err != nil {
		t.Fatalf("second RescheduleJob (reschedule): %v", err)
	}

	next := svc.NextRun(job)
	if next == nil {
		t.Error("expected non-nil next run after reschedule")
	}
}

func TestRunShellCommandEcho(t *testing.T) {
	svc, _ := New(nil, nil)
	exitCode, stdout, stderr := svc.runShellCommand("echo hello world", 5)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if stdout != "hello world" {
		t.Errorf("expected stdout 'hello world', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestRunShellCommandFailure(t *testing.T) {
	svc, _ := New(nil, nil)
	exitCode, _, _ := svc.runShellCommand("exit 42", 5)

	if exitCode != 1 {
		t.Errorf("expected exit code 1 (non-zero mapped to 1), got %d", exitCode)
	}
}

func TestRunShellCommandTimeout(t *testing.T) {
	svc, _ := New(nil, nil)
	exitCode, _, stderr := svc.runShellCommand("sleep 10", 1)

	if exitCode != -1 {
		t.Errorf("expected exit code -1 for timeout, got %d", exitCode)
	}
	if stderr != "command timed out" {
		t.Errorf("expected 'command timed out', got %q", stderr)
	}
}

func TestTriggerNowReturnsErrorWithNilStore(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(nil, nil)
	_, err := svc.TriggerNow(ctx, "nonexistent-job")
	if err == nil {
		t.Error("expected error when calling TriggerNow with nil store")
	}
}

func TestNewServiceLoggerDefault(t *testing.T) {
	svc, _ := New(nil, nil)
	if svc.logger == nil {
		t.Error("expected default logger when nil is passed")
	}
}

func TestMultipleEntriesNextRun(t *testing.T) {
	svc, _ := New(nil, nil)
	svc.cron.Start()
	defer svc.Stop()

	jobs := []store.CronJob{
		{ID: "job-1", Schedule: "*/1 * * * * *"},
		{ID: "job-2", Schedule: "*/2 * * * * *"},
		{ID: "job-3", Schedule: "*/3 * * * * *"},
	}

	for _, job := range jobs {
		svc.mu.Lock()
		eid, _ := svc.cron.AddFunc(job.Schedule, func() {})
		svc.entries[job.ID] = eid
		svc.mu.Unlock()
	}

	for _, job := range jobs {
		next := svc.NextRun(job)
		if next == nil {
			t.Errorf("expected next run for %s", job.ID)
		}
	}
}

func TestTriggerNowGeneratesExecutionID(t *testing.T) {
	svc, _ := New(nil, nil)
	id := svc.GenerateExecutionID()
	if len(id) == 0 {
		t.Error("expected a non-empty UUID string")
	}
}
