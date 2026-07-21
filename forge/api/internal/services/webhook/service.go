package webhook

import (
	"context"
	"strings"
	"sync"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Service struct {
	store *store.Store
	wg    sync.WaitGroup
}

func NewService(s *store.Store) *Service {
	return &Service{store: s}
}

func (s *Service) Wait() {
	if s != nil {
		s.wg.Wait()
	}
}

// Handle snapshots matching subscriptions into the durable delivery outbox.
func (s *Service) Handle(ctx context.Context, ev events.Envelope) error {
	if s.store == nil {
		return nil
	}

	s.wg.Add(1)
	defer s.wg.Done()

	payload := map[string]any{
		"id":             ev.ID,
		"timestamp":      ev.Timestamp,
		"source":         ev.Source,
		"resource_type":  ev.ResourceType,
		"resource_id":    ev.ResourceID,
		"correlation_id": ev.CorrelationID,
	}
	for k, v := range ev.Payload {
		payload[k] = v
	}

	eventName := formatEventName(ev.Type)
	return s.store.EnqueueWebhookEvent(ctx, eventName, payload)
}

func formatEventName(t events.EventType) string {
	name := string(t)
	if strings.Contains(name, ":") {
		return strings.ToLower(name)
	}

	prefixes := []string{
		"Server", "Node", "Placement", "Reservation", "DesiredState",
		"ActualState", "Evacuation", "Runtime", "Migration", "Recovery",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			action := strings.TrimPrefix(name, prefix)
			return strings.ToLower(prefix) + ":" + strings.ToLower(action)
		}
	}

	return strings.ToLower(name)
}
