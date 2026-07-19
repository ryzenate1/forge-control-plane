package health

import (
	"context"
	"time"
)

type QueueStatusFunc func() (active bool, lastTick time.Time, lastErr string)

type QueueCheck struct {
	label      string
	statusFunc QueueStatusFunc
}

func NewQueueCheck(label string, statusFunc QueueStatusFunc) *QueueCheck {
	return &QueueCheck{
		label:      label,
		statusFunc: statusFunc,
	}
}

func (c *QueueCheck) Name() string  { return "queue" }
func (c *QueueCheck) Label() string { return c.label }

func (c *QueueCheck) Run(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusWarning,
		Message: "Queue health check unavailable",
	}

	if c.statusFunc == nil {
		return result
	}

	active, lastTick, lastErr := c.statusFunc()
	details := map[string]any{
		"active": active,
	}
	if !lastTick.IsZero() {
		details["lastTick"] = lastTick.Format(time.RFC3339)
	}

	if lastErr != "" {
		result.Status = StatusFailed
		result.Message = lastErr
		details["lastError"] = lastErr
	} else if active {
		result.Status = StatusOK
		result.Message = "Queue worker is active"
	} else {
		result.Status = StatusWarning
		result.Message = "Queue worker is inactive"
	}

	result.Details = details
	return result
}
