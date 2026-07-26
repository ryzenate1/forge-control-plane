package contextbag

import (
	"context"
	"sync"
)

// Bag tracks cancelable background operations owned by the daemon.
type Bag struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func New() *Bag {
	return &Bag{cancels: make(map[string]context.CancelFunc)}
}

func (b *Bag) Add(key string, cancel context.CancelFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if previous := b.cancels[key]; previous != nil {
		previous()
	}
	b.cancels[key] = cancel
}

func (b *Bag) Remove(key string) {
	b.mu.Lock()
	delete(b.cancels, key)
	b.mu.Unlock()
}

func (b *Bag) Cancel(key string) {
	b.mu.Lock()
	cancel := b.cancels[key]
	delete(b.cancels, key)
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (b *Bag) CancelAll() {
	b.mu.Lock()
	cancels := b.cancels
	b.cancels = make(map[string]context.CancelFunc)
	b.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
