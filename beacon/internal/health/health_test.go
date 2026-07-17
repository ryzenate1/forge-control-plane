package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// MockHealthChecker is a mock implementation of HealthChecker for testing
type MockHealthChecker struct {
	shouldFail bool
}

func (m *MockHealthChecker) Check(ctx context.Context) error {
	if m.shouldFail {
		return errors.New("health check failed")
	}
	return nil
}

func TestCompositeHealthChecker(t *testing.T) {
	// Test with all checkers passing
	checkers := []HealthChecker{
		&MockHealthChecker{shouldFail: false},
		&MockHealthChecker{shouldFail: false},
	}
	composite := &CompositeHealthChecker{checkers: checkers}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := composite.Check(ctx); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test with one checker failing
	checkers = []HealthChecker{
		&MockHealthChecker{shouldFail: false},
		&MockHealthChecker{shouldFail: true},
		&MockHealthChecker{shouldFail: false},
	}
	composite = &CompositeHealthChecker{checkers: checkers}

	if err := composite.Check(ctx); err == nil {
		t.Error("Expected an error, got none")
	}
}
