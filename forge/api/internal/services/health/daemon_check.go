package health

import (
	"context"
)

// DaemonCheck summarizes persisted node heartbeat state. It does not perform
// live daemon connectivity probes; use a node probe when live reachability is
// required.
type DaemonCheck struct {
	nodeStatusFunc func(context.Context) (total, healthy, unhealthy int, details map[string]any, err error)
}

func NewDaemonCheck(nodeStatus func(context.Context) (int, int, int, map[string]any, error)) *DaemonCheck {
	return &DaemonCheck{
		nodeStatusFunc: nodeStatus,
	}
}

func (c *DaemonCheck) Name() string  { return "daemon" }
func (c *DaemonCheck) Label() string { return "Daemon Heartbeat Status" }

func (c *DaemonCheck) Run(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusWarning,
		Message: "No nodes registered for heartbeat monitoring",
	}

	if c.nodeStatusFunc == nil {
		return result
	}

	total, healthy, unhealthy, details, err := c.nodeStatusFunc(ctx)
	if err != nil {
		result.Status = StatusFailed
		result.Message = err.Error()
		return result
	}

	switch {
	case total == 0:
		result.Status = StatusWarning
		result.Message = "No nodes registered for heartbeat monitoring"
	case healthy == total:
		result.Status = StatusOK
		result.Message = "All persisted node heartbeats are healthy"
	case healthy > 0:
		result.Status = StatusWarning
		result.Message = "Some persisted node heartbeats are not healthy"
	default:
		result.Status = StatusFailed
		result.Message = "No persisted node heartbeats are healthy"
	}

	if details == nil {
		details = make(map[string]any)
	}
	details["totalNodes"] = total
	// onlineNodes and offlineNodes remain for existing consumers; they reflect
	// persisted heartbeat state, not a live connectivity probe.
	details["onlineNodes"] = healthy
	details["offlineNodes"] = unhealthy
	details["healthyHeartbeatNodes"] = healthy
	details["nonHealthyHeartbeatNodes"] = unhealthy
	details["stateSource"] = "persistedHeartbeatState"

	result.Details = details
	return result
}
