package health

import (
	"context"
	"time"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusFailed  Status = "failed"
)

type Check interface {
	Name() string
	Label() string
	Run(ctx context.Context) CheckResult
}

// CriticalCheck marks a dependency failure as making the API unready.
// Checks that do not implement it remain diagnostic-only for compatibility.
type CriticalCheck interface {
	Check
	Critical() bool
}

type CheckResult struct {
	Name                string         `json:"name"`
	Status              Status         `json:"status"`
	Label               string         `json:"label"`
	Message             string         `json:"notificationMessage"`
	Critical            bool           `json:"critical"`
	LatencyMs           int64          `json:"latencyMs,omitempty"`
	Details             map[string]any `json:"details,omitempty"`
	LastChecked         time.Time      `json:"lastChecked,omitempty"`
	LastSuccess         *time.Time     `json:"lastSuccess,omitempty"`
	LastFailure         *time.Time     `json:"lastFailure,omitempty"`
	ConsecutiveFailures int            `json:"consecutiveFailures,omitempty"`
}

type HealthReport struct {
	// Status is the diagnostic policy: failed if any check failed, warning if
	// none failed but at least one warned, and ok otherwise.
	Status    Status        `json:"status"`
	OK        bool          `json:"ok"`
	Service   string        `json:"service"`
	Version   string        `json:"version,omitempty"`
	Uptime    string        `json:"uptime,omitempty"`
	Checks    []CheckResult `json:"checks"`
	CheckedAt time.Time     `json:"checkedAt"`
}

func (r HealthReport) Ready() bool {
	for _, check := range r.Checks {
		if check.Critical && check.Status == StatusFailed {
			return false
		}
	}
	return true
}
