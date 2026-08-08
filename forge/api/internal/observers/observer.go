package observers

import (
	"context"
	"fmt"
	"sync"

	"gamepanel/forge/internal/events"
)

type Observer interface {
	Name() string
	Handle(ctx context.Context, event events.Envelope) error
}

type ObserverRegistry struct {
	mu        sync.RWMutex
	observers map[string][]Observer
}

func NewObserverRegistry() *ObserverRegistry {
	return &ObserverRegistry{
		observers: make(map[string][]Observer),
	}
}

func (r *ObserverRegistry) Register(eventType string, observer Observer) {
	if observer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.observers[eventType]) >= 128 {
		return
	}
	r.observers[eventType] = append(r.observers[eventType], observer)
}

func (r *ObserverRegistry) Unregister(eventType, observerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	observers := r.observers[eventType]
	filtered := observers[:0]
	for _, observer := range observers {
		if observer.Name() != observerName {
			filtered = append(filtered, observer)
		}
	}
	if len(filtered) == 0 {
		delete(r.observers, eventType)
		return
	}
	r.observers[eventType] = filtered
}

func (r *ObserverRegistry) Dispatch(ctx context.Context, event events.Envelope) error {
	r.mu.RLock()
	observers := append([]Observer(nil), r.observers[string(event.Type)]...)
	r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(observers))
	for _, obs := range observers {
		observer := obs
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs <- fmt.Errorf("observer %s panicked: %v", observer.Name(), recovered)
				}
			}()
			if err := observer.Handle(ctx, event); err != nil {
				errs <- fmt.Errorf("observer %s: %w", observer.Name(), err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	var firstError error
	for err := range errs {
		if firstError == nil {
			firstError = err
		}
	}
	return firstError
}
