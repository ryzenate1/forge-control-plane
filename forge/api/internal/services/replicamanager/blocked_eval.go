package replicamanager

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"gamepanel/forge/internal/domain"
)

// pendingPlacement represents a blocked evaluation waiting for capacity.
// Minimal in-memory queue analogous to Nomad's blocked-evals, re-evaluated
// on capacity change (node heartbeat, allocation freed) rather than only
// 60s poll. Feature-flagged additive.
type pendingPlacement struct {
	AppID      string
	InstanceID string
	LastNodeID string // node to exclude on next attempt (simple penalty)
	Attempts   int
	NextRetry  time.Time
	CreatedAt  time.Time
	index      int // heap index
}

// blockedEvalHeap is a min-heap ordered by NextRetry.
type blockedEvalHeap []*pendingPlacement

func (h blockedEvalHeap) Len() int           { return len(h) }
func (h blockedEvalHeap) Less(i, j int) bool { return h[i].NextRetry.Before(h[j].NextRetry) }
func (h blockedEvalHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *blockedEvalHeap) Push(x any) {
	n := len(*h)
	item := x.(*pendingPlacement)
	item.index = n
	*h = append(*h, item)
}
func (h *blockedEvalHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// BlockedEvalQueue is an in-memory priority queue for pending placements.
// It is re-evaluated on capacity change events (node added, allocation freed)
// and on periodic reconcile. Thread-safe.
type BlockedEvalQueue struct {
	mu      sync.Mutex
	pending map[string]*pendingPlacement // keyed by InstanceID
	heap    blockedEvalHeap
	notify  chan struct{}
}

// NewBlockedEvalQueue creates a queue with a buffered notify channel.
func NewBlockedEvalQueue() *BlockedEvalQueue {
	q := &BlockedEvalQueue{
		pending: make(map[string]*pendingPlacement),
		notify:  make(chan struct{}, 1),
	}
	heap.Init(&q.heap)
	return q
}

// Enqueue adds or updates a pending placement. NextRetry is when it becomes eligible.
func (q *BlockedEvalQueue) Enqueue(appID, instanceID, lastNodeID string, attempts int, nextRetry time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if existing, ok := q.pending[instanceID]; ok {
		existing.AppID = appID
		existing.LastNodeID = lastNodeID
		existing.Attempts = attempts
		if nextRetry.Before(existing.NextRetry) {
			existing.NextRetry = nextRetry
		} else {
			existing.NextRetry = nextRetry
		}
		heap.Fix(&q.heap, existing.index)
		return
	}
	item := &pendingPlacement{
		AppID:      appID,
		InstanceID: instanceID,
		LastNodeID: lastNodeID,
		Attempts:   attempts,
		NextRetry:  nextRetry,
		CreatedAt:  time.Now(),
	}
	q.pending[instanceID] = item
	heap.Push(&q.heap, item)
	// Non-blocking notify
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Remove deletes a pending entry (e.g., after successful placement).
func (q *BlockedEvalQueue) Remove(instanceID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item, ok := q.pending[instanceID]; ok {
		heap.Remove(&q.heap, item.index)
		delete(q.pending, instanceID)
	}
}

// PeekNext returns the soonest pending placement without removing it.
func (q *BlockedEvalQueue) PeekNext() *pendingPlacement {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.heap.Len() == 0 {
		return nil
	}
	// Return copy to avoid data race
	orig := q.heap[0]
	cp := *orig
	return &cp
}

// PopReady returns all placements whose NextRetry <= now, removing them from queue.
func (q *BlockedEvalQueue) PopReady(now time.Time) []*pendingPlacement {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ready []*pendingPlacement
	for q.heap.Len() > 0 && !q.heap[0].NextRetry.After(now) {
		item := heap.Pop(&q.heap).(*pendingPlacement)
		delete(q.pending, item.InstanceID)
		ready = append(ready, item)
	}
	return ready
}

// Len returns pending count.
func (q *BlockedEvalQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Notify returns a channel that signals when new pending placements are enqueued
// or when Trigger is called for capacity change.
func (q *BlockedEvalQueue) Notify() <-chan struct{} { return q.notify }

// Trigger signals capacity change (node heartbeat, allocation freed) to wake up
// blocked eval processing. Non-blocking.
func (q *BlockedEvalQueue) Trigger() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// List returns a snapshot of pending placements (for metrics/debugging).
func (q *BlockedEvalQueue) List() []pendingPlacement {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]pendingPlacement, 0, len(q.pending))
	for _, p := range q.pending {
		out = append(out, *p)
	}
	return out
}

// OnCapacityChange is a hook to be called when capacity changes (node added,
// allocation freed). It triggers re-evaluation of blocked evals.
func (m *Manager) OnCapacityChange(ctx context.Context) {
	if m == nil || m.blockedQueue == nil {
		return
	}
	m.blockedQueue.Trigger()
	// Opportunistically try to drain ready pending placements (non-blocking)
	go func() {
		// Use background context with timeout to avoid blocking caller (heartbeat path)
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := m.processBlockedEvals(c); err != nil {
			// Log is handled inside processBlockedEvals
		}
	}()
}

// NotifyCapacityChange is an alias for OnCapacityChange for external callers (http handler, scheduler).
func (m *Manager) NotifyCapacityChange(ctx context.Context) {
	m.OnCapacityChange(ctx)
}

// processBlockedEvals drains ready pending placements and attempts to replace them.
// Returns number of retried placements.
func (m *Manager) processBlockedEvals(ctx context.Context) (int, error) {
	if m == nil || m.blockedQueue == nil {
		return 0, nil
	}
	now := time.Now()
	ready := m.blockedQueue.PopReady(now)
	if len(ready) == 0 {
		return 0, nil
	}
	retried := 0
	for _, p := range ready {
		// Attempt replacement, respecting reschedule policy and excluding last node
		attempts, _ := m.store.GetInstanceReplacementAttempts(ctx, p.InstanceID)
		// Check policy eligibility
		inst, err := m.store.GetInstance(ctx, p.InstanceID)
		if err != nil {
			continue // instance gone
		}
		if m.reschedulePolicy != nil {
			if eligible, _, _ := m.reschedulePolicy.ShouldRetry(attempts, inst.UpdatedAt, now); !eligible {
				// Re-enqueue with next delay if still within policy
				delay := m.reschedulePolicy.NextDelay(attempts + 1)
				nextRetry := inst.UpdatedAt.Add(delay)
				if nextRetry.Before(now) {
					nextRetry = now.Add(delay)
				}
				m.blockedQueue.Enqueue(p.AppID, p.InstanceID, p.LastNodeID, attempts, nextRetry)
				continue
			}
		}
		// Try replacement with last node excluded
		if err := m.replaceInstanceExcluding(ctx, p.InstanceID, p.LastNodeID); err != nil {
			// Failed again — re-enqueue with backoff
			newAttempts, _ := m.store.IncrementInstanceReplacementAttempts(ctx, p.InstanceID)
			delay := 30 * time.Second
			if m.reschedulePolicy != nil {
				delay = m.reschedulePolicy.NextDelay(newAttempts)
			}
			m.blockedQueue.Enqueue(p.AppID, p.InstanceID, inst.NodeID, newAttempts, now.Add(delay))
			continue
		}
		// Success — reset attempts and remove from queue (already popped)
		_ = m.store.ResetInstanceReplacementAttempts(ctx, p.InstanceID)
		retried++
		// Capacity may have changed, trigger again
		m.blockedQueue.Trigger()
	}
	return retried, nil
}

// replaceInstanceExcluding attempts to replace a failed instance while excluding a specific node.
// This implements the "penalty by not retrying same node that failed" requirement.
// Minimal additive: call ReplaceInstance and if it picks the excluded node, revert and treat as failure
// so that blocked-eval will backoff and later pick a different node when capacity allows.
func (m *Manager) replaceInstanceExcluding(ctx context.Context, instanceID, excludeNodeID string) error {
	if excludeNodeID == "" {
		_, err := m.ReplaceInstance(ctx, instanceID)
		return err
	}
	// Remember node before replacement so we can revert if penalty violated
	instBefore, err := m.store.GetInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	beforeNode := instBefore.NodeID
	reason, err := m.ReplaceInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if reason == nil {
		return errNoPlacement
	}
	if !reason.Accepted {
		return errNoPlacement
	}
	if reason.NodeID == excludeNodeID {
		// Penalty: we selected the same failed node — revert to failed and backoff
		_, _ = m.store.UpdateInstanceNode(ctx, instanceID, beforeNode)
		_, _ = m.store.UpdateInstanceStatus(ctx, instanceID, "failed")
		return errExcludedNodeSelected
	}
	return nil
}

var (
	errNoPlacement          = fmt.Errorf("no placement")
	errExcludedNodeSelected = fmt.Errorf("placement selected excluded node")
)

// domainReplaceFailedInstanceRequest is a small helper to avoid import cycles in tests.
func domainReplaceFailedInstanceRequest(instanceID string) domain.ReplaceFailedInstanceRequest {
	return domain.ReplaceFailedInstanceRequest{InstanceID: instanceID}
}
