package health

import (
	"context"
	"time"
)

type CacheCheck struct {
	pingFunc func(context.Context) error
	infoFunc func(context.Context) (map[string]any, error)
	enabled  bool
}

func NewCacheCheck(ping func(context.Context) error, info func(context.Context) (map[string]any, error), enabled bool) *CacheCheck {
	return &CacheCheck{
		pingFunc: ping,
		infoFunc: info,
		enabled:  enabled,
	}
}

func (c *CacheCheck) Name() string   { return "cache" }
func (c *CacheCheck) Label() string  { return "Cache" }
func (c *CacheCheck) Critical() bool { return c.enabled }

func (c *CacheCheck) Run(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusWarning,
		Message: "Cache not enabled",
	}

	if !c.enabled || c.pingFunc == nil {
		return result
	}

	start := time.Now()
	if err := c.pingFunc(ctx); err != nil {
		result.Status = StatusFailed
		result.Message = err.Error()
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.Status = StatusOK
	result.Message = "Connected"
	result.LatencyMs = time.Since(start).Milliseconds()

	if c.infoFunc != nil {
		details, err := c.infoFunc(ctx)
		if err == nil {
			result.Details = details
		}
	}

	return result
}
