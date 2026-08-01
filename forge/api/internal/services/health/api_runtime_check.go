package health

import (
	"context"
	"time"
)

// APIRuntimeCheck reports process-local API runtime information. It is
// diagnostic-only: serving this health response is the API availability signal,
// while dependency availability is represented by their individual checks.
type APIRuntimeCheck struct {
	startTime time.Time
}

func NewAPIRuntimeCheck(startTime time.Time) *APIRuntimeCheck {
	return &APIRuntimeCheck{startTime: startTime}
}

func (c *APIRuntimeCheck) Name() string  { return "api" }
func (c *APIRuntimeCheck) Label() string { return "API Runtime" }

func (c *APIRuntimeCheck) Run(context.Context) CheckResult {
	return CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "API process is serving health diagnostics",
		Details: map[string]any{
			"uptimeSeconds": time.Since(c.startTime).Seconds(),
		},
	}
}
