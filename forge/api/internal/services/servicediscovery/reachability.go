package servicediscovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type ReachabilityVerifier struct {
	registry *Registry
	mu       sync.RWMutex
	results  map[string]*ReachabilityResult
	timeout  time.Duration
	now      func() time.Time
}

func NewReachabilityVerifier(registry *Registry) *ReachabilityVerifier {
	return &ReachabilityVerifier{
		registry: registry,
		results:  make(map[string]*ReachabilityResult),
		timeout:  5 * time.Second,
		now:      time.Now,
	}
}

func resultKey(sourceNodeID, targetNodeID, serviceName string) string {
	return sourceNodeID + "/" + targetNodeID + "/" + serviceName
}

func (v *ReachabilityVerifier) VerifyEndpoint(ctx context.Context, ep ServiceEndpoint) *ReachabilityResult {
	result := &ReachabilityResult{
		SourceNodeID: ep.NodeID,
		TargetNodeID: ep.NodeID,
		ServiceName:  ep.ServiceName,
		CheckedAt:    v.now(),
	}

	address := net.JoinHostPort(ep.Address.String(), fmt.Sprintf("%d", ep.Port))
	conn, err := (&net.Dialer{Timeout: v.timeout}).DialContext(ctx, string(ep.Protocol), address)
	if err == nil {
		conn.Close()
		result.Reachable = true
	} else if !isContextError(err) {
		result.Reachable = false
		result.Error = err.Error()
	}

	v.mu.Lock()
	v.results[resultKey(ep.NodeID, ep.NodeID, ep.ServiceName)] = result
	v.mu.Unlock()

	return result
}

func (v *ReachabilityVerifier) VerifyCrossNode(ctx context.Context, sourceNodeID, targetNodeID, serviceName string, address string) *ReachabilityResult {
	result := &ReachabilityResult{
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		ServiceName:  serviceName,
		CheckedAt:    v.now(),
	}

	conn, err := (&net.Dialer{Timeout: v.timeout}).DialContext(ctx, "tcp", address)
	if err == nil {
		conn.Close()
		result.Reachable = true
	} else if !isContextError(err) {
		result.Reachable = false
		result.Error = err.Error()
	}

	v.mu.Lock()
	v.results[resultKey(sourceNodeID, targetNodeID, serviceName)] = result
	v.mu.Unlock()

	return result
}

func (v *ReachabilityVerifier) Sweep(ctx context.Context) []ReachabilityResult {
	endpoints := v.registry.ListEndpoints(EndpointFilter{HealthyOnly: false})
	var results []ReachabilityResult

	for _, ep := range endpoints {
		if ep.Status == EndpointStatusDraining {
			continue
		}
		result := v.VerifyEndpoint(ctx, ep)
		results = append(results, *result)
	}

	return results
}

func (v *ReachabilityVerifier) GetResult(sourceNodeID, targetNodeID, serviceName string) (*ReachabilityResult, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	r, ok := v.results[resultKey(sourceNodeID, targetNodeID, serviceName)]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

func (v *ReachabilityVerifier) ClearResults() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.results = make(map[string]*ReachabilityResult)
}

func (v *ReachabilityVerifier) SetTimeout(timeout time.Duration) {
	v.timeout = timeout
}

func isContextError(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "operation was canceled") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "i/o timeout")
}
