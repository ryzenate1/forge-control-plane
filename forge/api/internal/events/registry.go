package events

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const WildcardEventType EventType = "*"

const (
	defaultRetryCount    = 3
	defaultRetryBaseWait = 100 * time.Millisecond
)

type Metrics struct {
	EventsPublishedTotal      uint64            `json:"events_published_total"`
	EventsDeliveredTotal      uint64            `json:"events_delivered_total"`
	EventHandlerFailuresTotal uint64            `json:"event_handler_failures_total"`
	EventsDeadLetteredTotal   uint64            `json:"events_dead_lettered_total"`
	EventsByType              map[string]uint64 `json:"events_by_type"`
}

type subscriberEntry struct {
	subscriber Subscriber
	maxRetries int
}

type failureRecord struct {
	count    int
	lastErr  string
	lastTime time.Time
}

type Registry struct {
	source      string
	mu          sync.RWMutex
	subscribers map[EventType][]subscriberEntry
	failures    map[string]map[string]*failureRecord
	metrics     Metrics
	maxRetries  int
}

func NewRegistry(source string) *Registry {
	if source == "" {
		source = "api"
	}
	return &Registry{
		source:      source,
		subscribers: map[EventType][]subscriberEntry{},
		failures:    map[string]map[string]*failureRecord{},
		metrics:     Metrics{EventsByType: map[string]uint64{}},
		maxRetries:  defaultRetryCount,
	}
}

func (r *Registry) Source() string {
	if r == nil || r.source == "" {
		return "api"
	}
	return r.source
}

func (r *Registry) Subscribe(eventType EventType, subscriber Subscriber) {
	if r == nil || subscriber == nil {
		return
	}
	if eventType == "" {
		eventType = WildcardEventType
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers[eventType] = append(r.subscribers[eventType], subscriberEntry{
		subscriber: subscriber,
		maxRetries: r.maxRetries,
	})
}

func (r *Registry) SubscribeWithRetries(eventType EventType, subscriber Subscriber, maxRetries int) {
	if r == nil || subscriber == nil {
		return
	}
	if eventType == "" {
		eventType = WildcardEventType
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers[eventType] = append(r.subscribers[eventType], subscriberEntry{
		subscriber: subscriber,
		maxRetries: maxRetries,
	})
}

func (r *Registry) Publish(ctx context.Context, event Envelope) error {
	if r == nil {
		return nil
	}
	if event.Source == "" {
		event.Source = r.Source()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	r.mu.RLock()
	entries := append([]subscriberEntry{}, r.subscribers[event.Type]...)
	entries = append(entries, r.subscribers[WildcardEventType]...)
	r.mu.RUnlock()

	r.mu.Lock()
	r.metrics.EventsPublishedTotal++
	r.metrics.EventsByType[string(event.Type)]++
	r.mu.Unlock()

	for _, entry := range entries {
		subscriberKey := fmt.Sprintf("%p", entry.subscriber)
		eventKey := event.ID

		if r.deadLettered(eventKey, subscriberKey) {
			continue
		}

		if err := r.handleWithRetry(ctx, entry, event, subscriberKey, eventKey); err != nil {
			r.mu.Lock()
			r.metrics.EventHandlerFailuresTotal++
			r.mu.Unlock()
			continue
		}
		r.mu.Lock()
		r.metrics.EventsDeliveredTotal++
		r.mu.Unlock()
	}
	return nil
}

func (r *Registry) handleWithRetry(ctx context.Context, entry subscriberEntry, event Envelope, subscriberKey, eventKey string) error {
	var lastErr error
	wait := defaultRetryBaseWait
	for attempt := 0; attempt <= entry.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
		}
		if err := entry.subscriber.Handle(ctx, event); err != nil {
			lastErr = err
			continue
		}
		r.clearFailure(eventKey, subscriberKey)
		return nil
	}
	r.recordFailure(eventKey, subscriberKey, lastErr)
	return lastErr
}

func (r *Registry) recordFailure(eventKey, subscriberKey string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures[eventKey] == nil {
		r.failures[eventKey] = map[string]*failureRecord{}
	}
	rec, ok := r.failures[eventKey][subscriberKey]
	if !ok {
		r.failures[eventKey][subscriberKey] = &failureRecord{
			count:   1,
			lastErr: err.Error(),
			lastTime: time.Now(),
		}
		r.metrics.EventsDeadLetteredTotal++
		return
	}
	rec.count++
	rec.lastErr = err.Error()
	rec.lastTime = time.Now()
}

func (r *Registry) clearFailure(eventKey, subscriberKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failures[eventKey] != nil {
		delete(r.failures[eventKey], subscriberKey)
		if len(r.failures[eventKey]) == 0 {
			delete(r.failures, eventKey)
		}
	}
}

func (r *Registry) deadLettered(eventKey, subscriberKey string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rec, ok := r.failures[eventKey]; ok {
		if _, ok := rec[subscriberKey]; ok {
			return true
		}
	}
	return false
}

func (r *Registry) Metrics() Metrics {
	if r == nil {
		return Metrics{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := r.metrics
	cp.EventsByType = make(map[string]uint64, len(r.metrics.EventsByType))
	for k, v := range r.metrics.EventsByType {
		cp.EventsByType[k] = v
	}
	return cp
}
