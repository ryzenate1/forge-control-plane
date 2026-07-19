package health

import (
	"context"
	"runtime"
	"time"
)

type ReadinessStatus string

const (
	ReadinessOK      ReadinessStatus = "ok"
	ReadinessWarning ReadinessStatus = "warning"
	ReadinessFailed  ReadinessStatus = "failed"
)

type ReadinessCheck interface {
	Name() string
	Check(ctx context.Context) (ReadinessStatus, error)
}

type ReadinessResult struct {
	Status    ReadinessStatus            `json:"status"`
	Checks    map[string]ReadinessDetail `json:"checks"`
	Timestamp time.Time                  `json:"timestamp"`
}

type ReadinessDetail struct {
	Status  ReadinessStatus `json:"status"`
	Message string          `json:"message"`
}

type ReadinessChecker struct {
	checks []ReadinessCheck
}

func NewReadinessChecker() *ReadinessChecker {
	return &ReadinessChecker{
		checks: make([]ReadinessCheck, 0),
	}
}

func (r *ReadinessChecker) AddCheck(check ReadinessCheck) {
	r.checks = append(r.checks, check)
}

func (r *ReadinessChecker) AggregateReadiness(ctx context.Context) (*ReadinessResult, error) {
	result := &ReadinessResult{
		Status:    ReadinessOK,
		Checks:    make(map[string]ReadinessDetail),
		Timestamp: time.Now(),
	}

	for _, check := range r.checks {
		status, err := check.Check(ctx)
		detail := ReadinessDetail{Status: status}
		if err != nil {
			detail.Message = err.Error()
		} else {
			detail.Message = string(status)
		}
		result.Checks[check.Name()] = detail

		if status == ReadinessFailed {
			result.Status = ReadinessFailed
		} else if status == ReadinessWarning && result.Status != ReadinessFailed {
			result.Status = ReadinessWarning
		}
	}

	return result, nil
}

type ReadinessDatabaseCheck struct {
	ping func(ctx context.Context) error
}

func NewReadinessDatabaseCheck(ping func(ctx context.Context) error) *ReadinessDatabaseCheck {
	return &ReadinessDatabaseCheck{ping: ping}
}

func (c *ReadinessDatabaseCheck) Name() string { return "database" }

func (c *ReadinessDatabaseCheck) Check(ctx context.Context) (ReadinessStatus, error) {
	if c.ping == nil {
		return ReadinessWarning, nil
	}
	if err := c.ping(ctx); err != nil {
		return ReadinessFailed, err
	}
	return ReadinessOK, nil
}

type ReadinessDaemonCheck struct {
	check func(ctx context.Context) error
}

func NewReadinessDaemonCheck(check func(ctx context.Context) error) *ReadinessDaemonCheck {
	return &ReadinessDaemonCheck{check: check}
}

func (c *ReadinessDaemonCheck) Name() string { return "daemon" }

func (c *ReadinessDaemonCheck) Check(ctx context.Context) (ReadinessStatus, error) {
	if c.check == nil {
		return ReadinessWarning, nil
	}
	if err := c.check(ctx); err != nil {
		return ReadinessFailed, err
	}
	return ReadinessOK, nil
}

type ReadinessMemoryCheck struct {
	thresholdBytes uint64
}

func NewReadinessMemoryCheck(thresholdMB uint64) *ReadinessMemoryCheck {
	return &ReadinessMemoryCheck{thresholdBytes: thresholdMB * 1024 * 1024}
}

func NewReadinessMemoryCheckBytes(thresholdBytes uint64) *ReadinessMemoryCheck {
	return &ReadinessMemoryCheck{thresholdBytes: thresholdBytes}
}

func (c *ReadinessMemoryCheck) Name() string { return "memory" }

func (c *ReadinessMemoryCheck) Check(_ context.Context) (ReadinessStatus, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	if c.thresholdBytes > 0 && mem.HeapAlloc > c.thresholdBytes {
		return ReadinessWarning, nil
	}
	return ReadinessOK, nil
}
