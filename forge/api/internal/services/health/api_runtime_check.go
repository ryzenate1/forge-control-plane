package health

import (
	"context"
	"runtime"
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
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "API process is serving health diagnostics",
		Details: map[string]any{
			"goroutines":     runtime.NumGoroutine(),
			"heapAllocBytes": mem.Alloc,
			"heapSysBytes":   mem.HeapSys,
			"goVersion":      runtime.Version(),
			"goArch":         runtime.GOARCH,
			"goOS":           runtime.GOOS,
			"uptimeSeconds":  time.Since(c.startTime).Seconds(),
		},
	}
}
