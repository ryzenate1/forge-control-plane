package health

import (
	"context"
	"errors"
	"testing"
)

func TestReadinessChecker_AllOK(t *testing.T) {
	rc := NewReadinessChecker()
	rc.AddCheck(&mockReadinessCheck{name: "db", status: ReadinessOK})
	rc.AddCheck(&mockReadinessCheck{name: "daemon", status: ReadinessOK})

	result, err := rc.AggregateReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReadinessOK {
		t.Fatalf("expected ok, got %s", result.Status)
	}
	if len(result.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(result.Checks))
	}
}

func TestReadinessChecker_Warning(t *testing.T) {
	rc := NewReadinessChecker()
	rc.AddCheck(&mockReadinessCheck{name: "db", status: ReadinessOK})
	rc.AddCheck(&mockReadinessCheck{name: "cache", status: ReadinessWarning})

	result, err := rc.AggregateReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReadinessWarning {
		t.Fatalf("expected warning, got %s", result.Status)
	}
}

func TestReadinessChecker_Failed(t *testing.T) {
	rc := NewReadinessChecker()
	rc.AddCheck(&mockReadinessCheck{name: "db", status: ReadinessOK})
	rc.AddCheck(&mockReadinessCheck{name: "daemon", status: ReadinessFailed, err: errors.New("connection refused")})

	result, err := rc.AggregateReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReadinessFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if result.Checks["daemon"].Message != "connection refused" {
		t.Fatalf("expected error message, got %s", result.Checks["daemon"].Message)
	}
}

func TestReadinessChecker_FailedOverridesWarning(t *testing.T) {
	rc := NewReadinessChecker()
	rc.AddCheck(&mockReadinessCheck{name: "cache", status: ReadinessWarning})
	rc.AddCheck(&mockReadinessCheck{name: "db", status: ReadinessFailed, err: errors.New("down")})

	result, err := rc.AggregateReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReadinessFailed {
		t.Fatalf("expected failed (overrides warning), got %s", result.Status)
	}
}

func TestReadinessChecker_Empty(t *testing.T) {
	rc := NewReadinessChecker()
	result, err := rc.AggregateReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReadinessOK {
		t.Fatalf("expected ok with no checks, got %s", result.Status)
	}
}

func TestReadinessDatabaseCheck_OK(t *testing.T) {
	check := NewReadinessDatabaseCheck(func(ctx context.Context) error {
		return nil
	})
	if check.Name() != "database" {
		t.Fatalf("expected name database, got %s", check.Name())
	}
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessOK {
		t.Fatalf("expected ok, got %s", status)
	}
}

func TestReadinessDatabaseCheck_Failed(t *testing.T) {
	check := NewReadinessDatabaseCheck(func(ctx context.Context) error {
		return errors.New("connection refused")
	})
	status, err := check.Check(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if status != ReadinessFailed {
		t.Fatalf("expected failed, got %s", status)
	}
}

func TestReadinessDatabaseCheck_NilPing(t *testing.T) {
	check := NewReadinessDatabaseCheck(nil)
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessWarning {
		t.Fatalf("expected warning for nil ping, got %s", status)
	}
}

func TestReadinessDaemonCheck_OK(t *testing.T) {
	check := NewReadinessDaemonCheck(func(ctx context.Context) error {
		return nil
	})
	if check.Name() != "daemon" {
		t.Fatalf("expected name daemon, got %s", check.Name())
	}
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessOK {
		t.Fatalf("expected ok, got %s", status)
	}
}

func TestReadinessDaemonCheck_Failed(t *testing.T) {
	check := NewReadinessDaemonCheck(func(ctx context.Context) error {
		return errors.New("daemon unreachable")
	})
	status, err := check.Check(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if status != ReadinessFailed {
		t.Fatalf("expected failed, got %s", status)
	}
}

func TestReadinessMemoryCheck_OK(t *testing.T) {
	check := NewReadinessMemoryCheck(0)
	if check.Name() != "memory" {
		t.Fatalf("expected name memory, got %s", check.Name())
	}
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessOK {
		t.Fatalf("expected ok with no threshold, got %s", status)
	}
}

func TestReadinessMemoryCheck_WithHighThreshold(t *testing.T) {
	check := NewReadinessMemoryCheck(999999)
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessOK {
		t.Fatalf("expected ok with high threshold, got %s", status)
	}
}

func TestReadinessMemoryCheck_WithLowThreshold(t *testing.T) {
	check := NewReadinessMemoryCheckBytes(1)
	status, err := check.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != ReadinessWarning {
		t.Fatalf("expected warning with 1-byte threshold, got %s", status)
	}
}

type mockReadinessCheck struct {
	name   string
	status ReadinessStatus
	err    error
}

func (m *mockReadinessCheck) Name() string { return m.name }
func (m *mockReadinessCheck) Check(_ context.Context) (ReadinessStatus, error) {
	return m.status, m.err
}
