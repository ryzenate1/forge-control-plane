package health

import (
	"context"
	"errors"
	"testing"
)

type mockCheck struct {
	name   string
	label  string
	status Status
}

func (m *mockCheck) Name() string  { return m.name }
func (m *mockCheck) Label() string { return m.label }
func (m *mockCheck) Run(ctx context.Context) CheckResult {
	return CheckResult{
		Name:    m.name,
		Label:   m.label,
		Status:  m.status,
		Message: "mock result",
	}
}

func TestHealthService_RunAll(t *testing.T) {
	svc := NewService("1.0.0-test")
	svc.AddCheck(&mockCheck{name: "db", label: "Database", status: StatusOK})
	svc.AddCheck(&mockCheck{name: "cache", label: "Cache", status: StatusOK})
	svc.AddCheck(&mockCheck{name: "broken", label: "Broken", status: StatusFailed})

	report := svc.RunAll(context.Background())

	if report.OK {
		t.Error("expected report to be not OK (broken check)")
	}

	if report.Service != "api" {
		t.Errorf("expected service 'api', got %q", report.Service)
	}

	if report.Version != "1.0.0-test" {
		t.Errorf("expected version '1.0.0-test', got %q", report.Version)
	}

	if len(report.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(report.Checks))
	}

	if report.Checks[2].Status != StatusFailed {
		t.Errorf("expected broken check to be failed, got %q", report.Checks[2].Status)
	}
}

func TestHealthService_RunCheck(t *testing.T) {
	svc := NewService("test")
	svc.AddCheck(&mockCheck{name: "db", label: "Database", status: StatusOK})

	result := svc.RunCheck(context.Background(), "db")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %q", result.Status)
	}

	result = svc.RunCheck(context.Background(), "nonexistent")
	if result != nil {
		t.Errorf("expected nil for nonexistent check, got %v", result)
	}
}

func TestDatabaseCheck(t *testing.T) {
	check := NewDatabaseCheck(
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) (map[string]any, error) {
			return map[string]any{"version": "16.0"}, nil
		},
	)

	result := check.Run(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %q", result.Status)
	}

	check = NewDatabaseCheck(
		func(ctx context.Context) error { return errors.New("connection refused") },
		nil,
	)

	result = check.Run(context.Background())
	if result.Status != StatusFailed {
		t.Errorf("expected StatusFailed, got %q", result.Status)
	}
}

func TestCacheCheck(t *testing.T) {
	check := NewCacheCheck(
		func(ctx context.Context) error { return nil },
		nil,
		true,
	)

	result := check.Run(context.Background())
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %q", result.Status)
	}

	check = NewCacheCheck(nil, nil, false)
	result = check.Run(context.Background())
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %q", result.Status)
	}
}
