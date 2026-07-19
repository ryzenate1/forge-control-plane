package health

import (
	"context"
	"time"
)

type DatabaseCheck struct {
	label       string
	pingFunc    func(context.Context) error
	detailsFunc func(context.Context) (map[string]any, error)
}

func NewDatabaseCheck(ping func(context.Context) error, details func(context.Context) (map[string]any, error)) *DatabaseCheck {
	return &DatabaseCheck{
		// This check is intentionally limited to the panel metadata store; it
		// does not represent any externally configured provisioning host.
		label:       "Panel PostgreSQL",
		pingFunc:    ping,
		detailsFunc: details,
	}
}

func (c *DatabaseCheck) Name() string   { return "database" }
func (c *DatabaseCheck) Label() string  { return c.label }
func (c *DatabaseCheck) Critical() bool { return true }

func (c *DatabaseCheck) Run(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "Panel PostgreSQL connected",
	}

	if c.pingFunc == nil {
		result.Status = StatusWarning
		result.Message = "Panel PostgreSQL is not configured"
		return result
	}

	if err := c.pingFunc(ctx); err != nil {
		result.Status = StatusFailed
		result.Message = err.Error()
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.LatencyMs = time.Since(start).Milliseconds()

	if c.detailsFunc != nil {
		details, err := c.detailsFunc(ctx)
		if err == nil {
			result.Details = details
		}
	}

	return result
}
