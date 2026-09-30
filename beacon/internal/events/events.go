package events

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// Event represents an event
type Event struct {
	Topic string      `json:"topic"`
	Data  interface{} `json:"data"`
}

// Bus is an event bus
type Bus struct {
	mu        sync.RWMutex
	listeners map[string][]chan []byte
	closed    bool
}

// NewBus creates a new event bus
func NewBus() *Bus {
	return &Bus{
		listeners: make(map[string][]chan []byte),
	}
}

// Publish publishes an event to all subscribers of the given topic. If a
// subscriber channel is full, it waits up to 10ms and then drops the oldest
// message from the channel to make room, matching the Wings SinkPool ring
// buffer pattern.
//
// Send/close invariant: Publish holds b.mu (RLock) for the entire duration
// of every send attempt below, and Unsubscribe holds b.mu (Lock) for the
// entire duration of its "remove from registry + close channel" operation.
// Since a read-lock and a write-lock on the same sync.RWMutex can never be
// held concurrently, Publish can never be sending on a channel that
// Unsubscribe is concurrently closing, which is what would otherwise cause
// a "send on closed channel" panic.
func (b *Bus) Publish(topic string, data interface{}) error {
	enc, err := json.Marshal(Event{Topic: topic, Data: data})
	if err != nil {
		return err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return errors.New("event bus is closed")
	}

	for _, ch := range b.listeners[topic] {
		select {
		case ch <- enc:
		default:
			select {
			case <-ch:
				select {
				case ch <- enc:
				default:
				}
			default:
			}
		}
	}
	return nil
}

// Subscribe subscribes to events
func (b *Bus) Subscribe(topic string) <-chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan []byte, 32)
	if b.closed {
		close(ch)
		return ch
	}
	b.listeners[topic] = append(b.listeners[topic], ch)
	return ch
}

func (b *Bus) SubscribeContext(ctx context.Context, topic string) <-chan []byte {
	ch := b.Subscribe(topic)
	go func() {
		<-ctx.Done()
		b.Unsubscribe(topic, ch)
	}()
	return ch
}

// Unsubscribe removes a channel from the given topic's listener list and closes
// it. If the channel is not found, this function is a no-op.
//
// Safety: this removal-and-close is done entirely under b.mu (Lock), and
// Publish holds b.mu (RLock) around its entire send attempt to every
// listener. Because those two lock modes are mutually exclusive on the
// same sync.RWMutex, Publish can never observe a channel mid-close (or
// send to one after it has been closed), so this can't trigger a "send on
// closed channel" panic. See the comment on Publish for the full invariant.
func (b *Bus) Unsubscribe(topic string, ch <-chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	listeners := b.listeners[topic]
	for i, l := range listeners {
		if l != ch {
			continue
		}
		// Maintain order: shift left, nil last element, truncate.
		copy(listeners[i:], listeners[i+1:])
		listeners[len(listeners)-1] = nil
		b.listeners[topic] = listeners[:len(listeners)-1]
		close(l)
		return
	}
}

// Destroy closes all channels
func (b *Bus) Destroy() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for _, channels := range b.listeners {
		for _, ch := range channels {
			close(ch)
		}
	}
	b.listeners = make(map[string][]chan []byte)
}
