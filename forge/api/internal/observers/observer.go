package observers

import (
	"context"
	"gamepanel/forge/internal/events"
)

type Observer interface {
	Name() string
	Handle(ctx context.Context, event events.Envelope) error
}

type ObserverRegistry struct {
	observers map[string][]Observer
}

func NewObserverRegistry() *ObserverRegistry {
	return &ObserverRegistry{
		observers: make(map[string][]Observer),
	}
}

func (r *ObserverRegistry) Register(eventType string, observer Observer) {
	r.observers[eventType] = append(r.observers[eventType], observer)
}

func (r *ObserverRegistry) Dispatch(ctx context.Context, event events.Envelope) error {
	observers := r.observers[string(event.Type)]
	for _, obs := range observers {
		if err := obs.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
