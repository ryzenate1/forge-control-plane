package health

import (
	"context"
	"time"
)

type DiskCheck struct {
	label   string
	path    string
	warnPct float64
	failPct float64
}

func NewDiskCheck(path string, warnPercent, failPercent float64) *DiskCheck {
	if warnPercent <= 0 {
		warnPercent = 85
	}
	if failPercent <= 0 {
		failPercent = 95
	}
	return &DiskCheck{
		label:   "Disk Usage",
		path:    path,
		warnPct: warnPercent,
		failPct: failPercent,
	}
}

func (c *DiskCheck) Name() string  { return "disk" }
func (c *DiskCheck) Label() string { return c.label }

func (c *DiskCheck) Run(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:   c.Name(),
		Label:  c.Label(),
		Status: StatusOK,
	}

	total, _, used, err := getDiskSpace(c.path)
	if err != nil {
		result.Status = StatusFailed
		result.Message = "Disk stat failed: " + err.Error()
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	var usagePct float64
	if total > 0 {
		usagePct = float64(used) / float64(total) * 100
	}

	details := map[string]any{
		"totalBytes":   total,
		"usedBytes":    used,
		"usagePercent": usagePct,
	}
	result.Details = details

	switch {
	case usagePct >= c.failPct:
		result.Status = StatusFailed
		result.Message = "Disk usage critical"
	case usagePct >= c.warnPct:
		result.Status = StatusWarning
		result.Message = "Disk usage warning"
	default:
		result.Message = "Disk usage healthy"
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}
