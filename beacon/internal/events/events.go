package events

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
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
// buffer pattern. All channel sends happen concurrently.
func (b *Bus) Publish(topic string, data interface{}) {
	if strings.Contains(topic, ":") {
		parts := strings.SplitN(topic, ":", 2)
		if len(parts) == 2 {
			topic = parts[0]
		}
	}

	enc, err := json.Marshal(Event{Topic: topic, Data: data})
	if err != nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(b.listeners[topic]))
	for _, ch := range b.listeners[topic] {
		go func(c chan []byte) {
			defer wg.Done()
			select {
			case c <- enc:
			case <-time.After(10 * time.Millisecond):
				// Channel is full after 10ms — drop the oldest message
				// and try again, acting as a ring buffer.
				select {
				case <-c:
					select {
					case c <- enc:
					default:
					}
				default:
				}
			}
		}(ch)
	}
	wg.Wait()
}

// Subscribe subscribes to events
func (b *Bus) Subscribe(topic string) <-chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan []byte, 32)
	b.listeners[topic] = append(b.listeners[topic], ch)
	return ch
}

// Unsubscribe removes a channel from the given topic's listener list and closes
// it. If the channel is not found, this function is a no-op.
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
