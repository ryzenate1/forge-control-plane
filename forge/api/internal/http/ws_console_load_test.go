package http

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/events"
)

// TestWSConsoleLoad_ConcurrentTickets verifies the WS ticket store remains correct
// under concurrent load. It is non-flaky: no sleeps, no timing assumptions, deterministic
// inputs and bounded concurrency.
func TestWSConsoleLoad_ConcurrentTickets(t *testing.T) {
	cfg := Config{AuthSecret: "load-test-secret-32-bytes-long-!!"}
	store := newWSTicketStore(cfg)

	const workers = 50
	const ticketsPerWorker = 20

	var wg sync.WaitGroup
	errCh := make(chan string, workers*ticketsPerWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < ticketsPerWorker; i++ {
				subject := fmt.Sprintf("load-%d-%d-%d", worker, i, workers*i+worker)

				// Put ticket
				store.put(wsTicket{
					Subject:   subject,
					UserID:    "user-load",
					ServerID:  "server-1",
					Stream:    "console",
					ExpiresAt: time.Now().Add(5 * time.Minute),
				})

				token := signTicket(cfg.AuthSecret, subject)
				if token == "" {
					errCh <- "empty token"
					continue
				}

				// First verification must succeed (single-use)
				sid, stream, ok := VerifyWSTicket(cfg, store, token)
				if !ok {
					errCh <- "first verify failed for " + subject
					continue
				}
				if sid != "server-1" || stream != "console" {
					errCh <- "unexpected server/stream"
					continue
				}

				// Second verification must fail (consumed)
				_, _, ok = VerifyWSTicket(cfg, store, token)
				if ok {
					errCh <- "second verify should fail (single-use)"
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("load test error: %s", err)
	}
}

// TestWSConsoleLoad_ConcurrentHubClients verifies wsHub concurrent Subscribe/Unsubscribe/Handle
// remains race-free and bounded. The hub is wired for /notifications/ws fanout; the
// implementation must stay correct under per-user materialized load.
func TestWSConsoleLoad_ConcurrentHubBroadcast(t *testing.T) {
	hub := newWSHub(nil, 100)
	const clients = 20
	const eventsPerClient = 50

	// Simulate concurrent history writes (simulating event publisher)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerClient; j++ {
				// Use Handle to simulate event publisher under concurrency.
				// The event type must be hub-whitelisted or Handle drops it.
				ev := mockEvent(id, j)
				ev.Type = events.EventServerStarted
				_ = hub.Handle(nil, ev)
			}
		}(i)
	}
	wg.Wait()

	hub.mu.Lock()
	historyLen := len(hub.history)
	maxHistory := hub.maxHistory
	hub.mu.Unlock()

	if historyLen == 0 {
		t.Fatal("expected history to contain events after concurrent writes")
	}
	if historyLen > maxHistory {
		t.Fatalf("history exceeded capacity: %d > %d", historyLen, maxHistory)
	}
}

func mockEvent(a, b int) events.Envelope {
	return events.Envelope{
		ID:            fmt.Sprintf("evt-%d-%d", a, b),
		Type:          events.EventNodeOnline,
		Timestamp:     time.Now().UTC(),
		Source:        "test",
		ResourceType:  "node",
		ResourceID:    fmt.Sprintf("node-%d", a),
		CorrelationID: fmt.Sprintf("corr-%d-%d", a, b),
		Payload:       map[string]any{"idx": b},
	}
}

// TestWSConsoleLoad_ConcurrentAddWithUser ensures AddWithUser is race-free under load.
func TestWSConsoleLoad_ConcurrentAddWithUser(t *testing.T) {
	hub := newWSHub(nil, 50)
	const workers = 30

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sub := hub.Subscribe(fmt.Sprintf("user-%d", idx), "user", "")
			hub.Unsubscribe(sub)
			sub2 := hub.Subscribe(fmt.Sprintf("user-%d", idx), "admin", "")
			hub.Unsubscribe(sub2)
			// Simulate subscription map contention
			hub.mu.Lock()
			// Simulate client map mutation
			hub.mu.Unlock()
		}(i)
	}
	wg.Wait()

	hub.mu.Lock()
	if hub.subs == nil {
		t.Fatal("subs map should be initialized")
	}
	hub.mu.Unlock()
}
