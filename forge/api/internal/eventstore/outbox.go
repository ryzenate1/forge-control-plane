package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
)

const defaultBatchSize = 100

type Relay struct {
	store         *EventStore
	pollInterval  time.Duration
	subscribers   []func(context.Context, events.Envelope) error
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	started       bool
	maxRetries    int
	eventTimeout  time.Duration
}

func NewRelay(store *EventStore, pollInterval time.Duration) *Relay {
	return &Relay{
		store:        store,
		pollInterval: pollInterval,
		maxRetries:   3,
		eventTimeout: 30 * time.Second,
	}
}

func (r *Relay) Subscribe(handler func(context.Context, events.Envelope) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers = append(r.subscribers, handler)
}

func (r *Relay) Start(ctx context.Context) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	ctx, r.cancel = context.WithCancel(ctx)
	r.mu.Unlock()

	r.wg.Add(1)
	go r.pollLoop(ctx)
}

func (r *Relay) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()

	r.mu.Lock()
	r.started = false
	r.mu.Unlock()
}

func (r *Relay) pollLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

func (r *Relay) processBatch(ctx context.Context) {
	pending, err := r.store.Pending(ctx, defaultBatchSize)
	if err != nil {
		return
	}

	for _, stored := range pending {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := r.processEvent(ctx, stored); err != nil {
			return
		}
	}
}

func (r *Relay) processEvent(ctx context.Context, stored StoredEvent) error {
	eventCtx, eventCancel := context.WithTimeout(ctx, r.eventTimeout)
	defer eventCancel()

	var payload map[string]any
	if stored.Payload != "" {
		if err := json.Unmarshal([]byte(stored.Payload), &payload); err != nil {
			payload = map[string]any{}
		}
	} else {
		payload = map[string]any{}
	}

	envelope := events.Envelope{
		ID:            stored.ID,
		Type:          events.EventType(stored.Type),
		Timestamp:     stored.CreatedAt,
		Source:        stored.Source,
		ResourceType:  stored.ResourceType,
		ResourceID:    stored.ResourceID,
		CorrelationID: stored.CorrelationID,
		Payload:       payload,
	}

	r.mu.RLock()
	subs := make([]func(context.Context, events.Envelope) error, len(r.subscribers))
	copy(subs, r.subscribers)
	r.mu.RUnlock()

	if err := r.deliverWithRetries(eventCtx, subs, envelope, stored); err != nil {
		return err
	}

	if err := r.store.MarkDispatched(ctx, stored.ID); err != nil {
		return fmt.Errorf("relay mark dispatched %s: %w", stored.ID, err)
	}
	return nil
}

func (r *Relay) deliverWithRetries(ctx context.Context, subs []func(context.Context, events.Envelope) error, envelope events.Envelope, stored StoredEvent) error {
	var lastErr error
	wait := 100 * time.Millisecond

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
		}

		success := true
		for _, handler := range subs {
			if err := handler(ctx, envelope); err != nil {
				lastErr = fmt.Errorf("handler error for event %s: %w", envelope.ID, err)
				success = false
				break
			}
		}

		if success {
			return nil
		}
	}

	errStr := lastErr.Error()
	if err := r.store.MarkFailed(ctx, stored.ID, errStr); err != nil {
		return fmt.Errorf("relay mark failed %s: %w", stored.ID, err)
	}

	failureCount := stored.FailureCount + (r.maxRetries + 1)
	if failureCount >= maxFailureCount {
		if err := r.store.MoveToDeadLetter(ctx, stored.ID); err != nil {
			return fmt.Errorf("relay dead letter %s: %w", stored.ID, err)
		}
	}

	return lastErr
}
