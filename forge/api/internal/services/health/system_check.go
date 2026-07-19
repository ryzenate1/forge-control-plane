package health

import (
	"context"
	"runtime"
	"time"
)

type SystemCheck struct {
	startTime time.Time
}

func NewSystemCheck(startTime time.Time) *SystemCheck {
	return &SystemCheck{startTime: startTime}
}

func (c *SystemCheck) Name() string  { return "system" }
func (c *SystemCheck) Label() string { return "System" }

func (c *SystemCheck) Run(ctx context.Context) CheckResult {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "System healthy",
		Details: map[string]any{
			"goroutines":    runtime.NumGoroutine(),
			"heapAllocMb":   mem.Alloc / 1024 / 1024,
			"heapSysMb":     mem.HeapSys / 1024 / 1024,
			"goVersion":     runtime.Version(),
			"goArch":        runtime.GOARCH,
			"goOS":          runtime.GOOS,
			"uptimeSeconds": time.Since(c.startTime).Seconds(),
		},
	}
}
