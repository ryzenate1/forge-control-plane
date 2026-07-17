package health

import "context"

// HealthChecker defines an interface for health checks
type HealthChecker interface {
	Check(ctx context.Context) error
}

// CompositeHealthChecker combines multiple health checkers
type CompositeHealthChecker struct {
	checkers []HealthChecker
}

// Check runs all health checks and returns the first error encountered
func (c *CompositeHealthChecker) Check(ctx context.Context) error {
	for _, checker := range c.checkers {
		if err := checker.Check(ctx); err != nil {
			return err
		}
	}
	return nil
}
