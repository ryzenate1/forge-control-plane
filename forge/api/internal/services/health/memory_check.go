package health

import (
	"context"
	"runtime"
)

// MemoryCheck reports Go heap usage. A zero threshold deliberately means that
// usage is informational; Go heap allocation alone cannot determine host or
// container memory pressure without a configured limit.
type MemoryCheck struct {
	thresholdBytes uint64
}

func NewMemoryCheck(thresholdBytes uint64) *MemoryCheck {
	return &MemoryCheck{thresholdBytes: thresholdBytes}
}

func (c *MemoryCheck) Name() string  { return "memory" }
func (c *MemoryCheck) Label() string { return "API Memory" }

func (c *MemoryCheck) Run(context.Context) CheckResult {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "Memory usage is informational; no limit configured",
		Details: map[string]any{
			"heapAllocBytes": mem.Alloc,
			"heapSysBytes":   mem.HeapSys,
		},
	}
	if c.thresholdBytes == 0 {
		return result
	}

	result.Details["thresholdBytes"] = c.thresholdBytes
	if mem.Alloc > c.thresholdBytes {
		result.Status = StatusWarning
		result.Message = "Go heap allocation exceeds configured threshold"
		return result
	}
	result.Message = "Go heap allocation is within configured threshold"
	return result
}
