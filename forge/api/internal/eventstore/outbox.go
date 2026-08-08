package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
)

const defaultBatchSize = 10

type Relay struct {
	store        *EventStore
	pollInterval time.Duration
	subscribers  []func(context.Context, events.Envelope) error
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	started      bool
	maxRetries   int
	eventTimeout time.Duration
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
	defer func() {
		if r := recover(); r != nil {
			slog.Error("event relay poll loop panicked", "panic", r)
		}
	}()

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	retentionTicker := time.NewTicker(time.Hour)
	defer retentionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processBatch(ctx)
		case <-retentionTicker.C:
			now := time.Now().UTC()
			if err := r.store.Prune(ctx, now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
				slog.Warn("event relay retention failed", "error", err)
			}
		}
	}
}

func (r *Relay) processBatch(ctx context.Context) {
	pending, err := r.store.ClaimPending(ctx, defaultBatchSize, uuid.NewString(), time.Duration(defaultBatchSize+1)*r.eventTimeout)
	if err != nil {
		slog.Warn("event relay claim failed", "error", err)
		return
	}

	for _, stored := range pending {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := r.processEvent(ctx, stored); err != nil {
			slog.Error("event relay process failed", "event", stored.ID, "error", err)
		}
	}
}

func (r *Relay) processEvent(ctx context.Context, stored StoredEvent) error {
	eventCtx, eventCancel := context.WithTimeout(ctx, r.eventTimeout)
	defer eventCancel()

	var payload map[string]any
	if stored.Payload != "" {
		if err := json.Unmarshal([]byte(stored.Payload), &payload); err != nil {
			if markErr := r.markFailedOrDeadLetter(ctx, stored, fmt.Errorf("invalid event payload: %w", err)); markErr != nil {
				return markErr
			}
			return fmt.Errorf("invalid payload for event %s: %w", stored.ID, err)
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

	var markErr error
	for attempt := 0; attempt < 3; attempt++ {
		if markErr = r.store.MarkDispatched(ctx, stored.ID); markErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
	return fmt.Errorf("relay mark dispatched %s: %w", stored.ID, markErr)
}

func (r *Relay) deliverWithRetries(ctx context.Context, subs []func(context.Context, events.Envelope) error, envelope events.Envelope, stored StoredEvent) error {
	var lastErr error
	for _, handler := range subs {
		wait := 100 * time.Millisecond
		delivered := false
		for attempt := 0; attempt <= r.maxRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
				wait *= 2
			}
			if err := handler(ctx, envelope); err != nil {
				lastErr = fmt.Errorf("handler error for event %s: %w", envelope.ID, err)
				continue
			}
			delivered = true
			break
		}
		if !delivered && lastErr == nil {
			lastErr = fmt.Errorf("handler failed for event %s", envelope.ID)
		}
	}

	if lastErr == nil {
		return nil
	}
	if err := r.markFailedOrDeadLetter(ctx, stored, lastErr); err != nil {
		return err
	}
	return lastErr
}

func (r *Relay) markFailedOrDeadLetter(ctx context.Context, stored StoredEvent, failure error) error {
	if err := r.store.MarkFailed(ctx, stored.ID, failure.Error()); err != nil {
		return fmt.Errorf("relay mark failed %s: %w", stored.ID, err)
	}
	if stored.FailureCount+1 >= maxFailureCount {
		if err := r.store.MoveToDeadLetter(ctx, stored.ID); err != nil {
			return fmt.Errorf("relay dead letter %s: %w", stored.ID, err)
		}
	}
	return nil
}
