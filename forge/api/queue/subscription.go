package queue

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
)

type EventKind string

const (
	EventKindJobCompleted EventKind = "job_completed"
	EventKindJobFailed    EventKind = "job_failed"
	EventKindJobCancelled EventKind = "job_cancelled"
	EventKindJobSnoozed   EventKind = "job_snoozed"
)

type Event struct {
	Kind  EventKind
	Job   *JobRow
	Queue string
}

type Subscription struct {
	ch    chan *Event
	kinds map[EventKind]struct{}
	once  sync.Once
}

func newSubscription(bufSize int, kinds []EventKind) *Subscription {
	filter := make(map[EventKind]struct{}, len(kinds))
	for _, kind := range kinds {
		filter[kind] = struct{}{}
	}
	return &Subscription{
		ch:    make(chan *Event, bufSize),
		kinds: filter,
	}
}

func (s *Subscription) C() <-chan *Event {
	return s.ch
}

func (s *Subscription) Close() {
	s.once.Do(func() {
		close(s.ch)
	})
}

type SubscriptionManager struct {
	mu            sync.RWMutex
	subscriptions []*Subscription
}

func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{}
}

func (m *SubscriptionManager) Subscribe(kinds ...EventKind) *Subscription {
	sub := newSubscription(100, kinds)
	m.mu.Lock()
	m.subscriptions = append(m.subscriptions, sub)
	m.mu.Unlock()
	return sub
}

func (m *SubscriptionManager) Unsubscribe(sub *Subscription) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.subscriptions {
		if s == sub {
			m.subscriptions = append(m.subscriptions[:i], m.subscriptions[i+1:]...)
			sub.Close()
			return
		}
	}
}

func (m *SubscriptionManager) Publish(ctx context.Context, event *Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sub := range m.subscriptions {
		if len(sub.kinds) > 0 {
			if _, ok := sub.kinds[event.Kind]; !ok {
				continue
			}
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "publish panic recovered",
						"panic", r,
						"stack", string(debug.Stack()),
					)
				}
			}()
			select {
			case sub.ch <- event:
			case <-ctx.Done():
			}
		}()
	}
}

func (m *SubscriptionManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sub := range m.subscriptions {
		sub.Close()
	}
	m.subscriptions = nil
}
