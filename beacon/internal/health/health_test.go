package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCompositeHealthChecker(t *testing.T) {
	checkers := []ComponentHealthChecker{
		{Name: "ok1", Checker: func(ctx context.Context) error { return nil }},
		{Name: "ok2", Checker: func(ctx context.Context) error { return nil }},
	}
	composite := &CompositeHealthChecker{checkers: checkers}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := composite.Check(ctx); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	checkers = []ComponentHealthChecker{
		{Name: "ok", Checker: func(ctx context.Context) error { return nil }},
		{Name: "fail", Checker: func(ctx context.Context) error { return errors.New("health check failed") }},
		{Name: "ok2", Checker: func(ctx context.Context) error { return nil }},
	}
	composite = &CompositeHealthChecker{checkers: checkers}

	if err := composite.Check(ctx); err == nil {
		t.Error("Expected an error, got none")
	}
}
