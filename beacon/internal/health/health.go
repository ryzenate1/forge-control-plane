package health

import "context"

// HealthChecker defines an interface for health checks
type HealthChecker interface {
	Check(ctx context.Context) error
}

// ComponentHealthChecker associates a name with a check function
type ComponentHealthChecker struct {
	Name    string
	Checker func(ctx context.Context) error
}

// HealthStatus represents the result of a composite health check
type HealthStatus struct {
	Status     string             `json:"status"`
	Components []ComponentResult `json:"components,omitempty"`
}

// ComponentResult is the result of a single health component
type ComponentResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// CompositeHealthChecker combines multiple health checkers
type CompositeHealthChecker struct {
	checkers []ComponentHealthChecker
}

// NewCompositeHealthChecker creates a new composite health checker
func NewCompositeHealthChecker(checkers []ComponentHealthChecker) *CompositeHealthChecker {
	return &CompositeHealthChecker{checkers: checkers}
}

// Check runs all health checks and returns the first error encountered
func (c *CompositeHealthChecker) Check(ctx context.Context) error {
	for _, checker := range c.checkers {
		if err := checker.Checker(ctx); err != nil {
			return err
		}
	}
	return nil
}

// CheckStatus runs all health checks and returns a structured status with per-component results
func (c *CompositeHealthChecker) CheckStatus(ctx context.Context) HealthStatus {
	status := HealthStatus{Status: "healthy", Components: make([]ComponentResult, 0, len(c.checkers))}
	for _, checker := range c.checkers {
		cr := ComponentResult{Name: checker.Name, Status: "ok"}
		if err := checker.Checker(ctx); err != nil {
			cr.Status = "error"
			cr.Error = err.Error()
			status.Status = "unhealthy"
		}
		status.Components = append(status.Components, cr)
	}
	return status
}
