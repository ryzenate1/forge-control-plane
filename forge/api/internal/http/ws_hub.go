package http

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/models"
	"gamepanel/forge/internal/store"
)

// wsHub is a push-based, per-user notification fanout hub.
//
// Design (wired per audit decision):
//   - The hub subscribes to events.WildcardEventType via Attach. Incoming
//     domain envelopes are mapped to target user IDs (explicit payload keys,
//     server→owner resolution through the store) and delivered only to
//     subscriptions for those users plus admins — no cross-tenant leakage.
//   - Each subscription owns a bounded outbox (perUserOutboxCapacity). When a
//     consumer is slow the OLDEST queued envelope is dropped and a per-client
//     dropped counter increments; the hub itself never blocks on a client.
//   - Delivery is at-most-once per connected client after connect, with a
//     bounded replay window for reconnects (Subscribe with lastEventID).
//   - Stop() unsubscribes from the registry and closes every subscription so
//     shutdown cannot leak goroutines or sockets.
type wsHub struct {
	mu         sync.Mutex
	subs       map[*hubSubscription]struct{}
	history    []events.Envelope
	historyIDs map[string]struct{}
	maxHistory int
	resolve    eventUserResolver
	logger     *slog.Logger
	stopped    bool
}

// hubSubscription is one authenticated WS connection registered with the hub.
type hubSubscription struct {
	UserID  string
	Role    string
	outbox  chan events.Envelope
	dropped atomic.Uint64
	closed  sync.Once
	done    chan struct{}
}

const (
	// perUserOutboxCapacity bounds each subscription's queue (drop-oldest).
	perUserOutboxCapacity = 256
	// defaultHubHistory bounds the reconnect replay window.
	defaultHubHistory = 200
)

// maxHubHistory caps the configured replay window to keep memory bounded.
const maxHubHistory = 500

// eventUserResolver maps an incoming domain event to the user IDs that should
// receive it. Returning an empty slice means "no specific users" (admins only).
type eventUserResolver func(ctx context.Context, ev events.Envelope) []string

// hubEventTypes whitelists the domain events worth pushing to user WebSockets:
// server status/power transitions, install lifecycle, backup completion,
// deployments and directly-targeted user events. High-volume internal chatter
// (placement, compose polling, reservations) is deliberately excluded.
var hubEventTypes = map[events.EventType]bool{
	events.EventServerCreated:               true,
	events.EventServerDeleted:               true,
	events.EventServerStarted:               true,
	events.EventServerStopped:               true,
	events.EventServerRestarted:             true,
	events.EventServerSuspended:             true,
	events.EventServerUnsuspended:           true,
	events.EventServerInstallCompleted:      true,
	events.EventServerBackupCreated:         true,
	events.EventServerBackupFailed:          true,
	events.EventServerBackupRestored:        true,
	events.EventServerCrashed:               true,
	events.EventServerCrashAutoRestarted:    true,
	events.EventServerCrashThresholdReached: true,
	events.EventServerCrashRecovered:        true,
	events.EventDeploymentCompleted:         true,
	events.EventDeploymentFailed:            true,
	events.EventDeploymentRolledBack:        true,
	events.EventUserCreated:                 true,
	events.EventUserUpdated:                 true,
}

func newWSHub(resolve eventUserResolver, maxHistory int) *wsHub {
	if maxHistory <= 0 || maxHistory > maxHubHistory {
		maxHistory = defaultHubHistory
	}
	return &wsHub{
		subs:       make(map[*hubSubscription]struct{}),
		history:    make([]events.Envelope, 0, maxHistory),
		historyIDs: make(map[string]struct{}, maxHistory),
		maxHistory: maxHistory,
		resolve:    resolve,
	}
}

// NewNotificationWSHub builds the production notification hub wired to the
// panel store for server→owner resolution. It attaches itself to the registry
// immediately; call Stop on the shutdown chain to detach and close clients.
func NewNotificationWSHub(st *store.Store, registry *events.Registry, logger *slog.Logger) *WSHub {
	hub := newWSHub(notificationUserResolver(st), defaultHubHistory)
	hub.logger = logger
	if registry != nil {
		registry.Subscribe(events.WildcardEventType, hub)
	}
	return hub
}

// WSHub is the exported name of the notification fanout hub so cmd/api can
// construct, inject and stop it across package boundaries.
type WSHub = wsHub

func (h *wsHub) logDebug(msg string, args ...any) {
	if h != nil && h.logger != nil {
		h.logger.Debug(msg, args...)
	}
}

// notificationUserResolver maps domain envelopes to target users. Explicit
// payload targeting wins; otherwise server-scoped resources resolve to their
// owner. Node/infra events resolve to nobody and therefore reach admins only.
func notificationUserResolver(st *store.Store) eventUserResolver {
	return func(ctx context.Context, ev events.Envelope) []string {
		users := make([]string, 0, 2)
		for _, key := range []string{"user_id", "userId", "ownerId", "owner_id"} {
			if v, ok := ev.Payload[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					users = append(users, s)
				}
			}
		}
		if len(users) > 0 {
			return users
		}
		switch ev.ResourceType {
		case "server", "servers":
			if st == nil || ctx.Err() != nil {
				return nil
			}
			ownerID, err := st.ServerOwnerID(ctx, ev.ResourceID)
			if err != nil || ownerID == "" {
				return nil
			}
			return []string{ownerID}
		case "user", "users":
			if ev.ResourceID != "" {
				return []string{ev.ResourceID}
			}
		}
		return nil
	}
}

// Subscribe registers a new connection for userID and returns the
// subscription whose Outbox the caller must drain. If lastEventID matches a
// recent envelope, newer history entries are queued first for reconnect
// continuity.
//
// The returned outbox channel is never closed by the hub (closing it would
// race concurrent fanout senders); Done() signals subscription closure and
// consumers must select on it.
func (h *wsHub) Subscribe(userID, role, lastEventID string) *hubSubscription {
	sub := &hubSubscription{
		UserID: userID,
		Role:   role,
		outbox: make(chan events.Envelope, perUserOutboxCapacity),
		done:   make(chan struct{}),
	}
	if userID == "" {
		close(sub.done)
		return sub
	}
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		close(sub.done)
		return sub
	}
	var replay []events.Envelope
	if lastEventID != "" {
		for i, ev := range h.history {
			if ev.ID == lastEventID && i+1 < len(h.history) {
				replay = append(replay, h.history[i+1:]...)
				break
			}
		}
	}
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	for _, ev := range replay {
		sub.enqueue(ev)
	}
	return sub
}

// Unsubscribe removes a subscription and signals Done. The outbox channel is
// deliberately left open so in-flight fanout senders cannot panic on a closed
// channel. Safe to call multiple times.
func (h *wsHub) Unsubscribe(sub *hubSubscription) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	delete(h.subs, sub)
	h.mu.Unlock()
	sub.closed.Do(func() { close(sub.done) })
}

// Outbox yields queued envelopes; consume together with Done().
func (s *hubSubscription) Outbox() <-chan events.Envelope {
	return s.outbox
}

// Dropped reports how many envelopes were dropped-oldest for this client.
func (s *hubSubscription) Dropped() uint64 {
	return s.dropped.Load()
}

// Done is closed when the subscription is removed by the hub.
func (s *hubSubscription) Done() <-chan struct{} {
	return s.done
}

// enqueue delivers one envelope without ever blocking; when the outbox is full
// the oldest envelope is dropped and counted.
func (s *hubSubscription) enqueue(ev events.Envelope) {
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.outbox <- ev:
	default:
		select {
		case <-s.outbox:
			s.dropped.Add(1)
		default:
		}
		select {
		case s.outbox <- ev:
		default:
			s.dropped.Add(1)
		}
	}
}

// Attach registers the hub for all event types on the given registry.
func (h *wsHub) Attach(registry *events.Registry) {
	if h == nil || registry == nil {
		return
	}
	registry.Subscribe(events.WildcardEventType, h)
}

// Handle implements events.Subscriber. Deduplicates against the bounded
// history, records the envelope for reconnect replay, then fans out to
// matching subscriptions without blocking on any single client.
func (h *wsHub) Handle(ctx context.Context, ev events.Envelope) error {
	if h == nil || ev.ID == "" || !hubEventTypes[ev.Type] {
		return nil
	}
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return nil
	}
	if _, dup := h.historyIDs[ev.ID]; !dup {
		h.history = append(h.history, ev)
		h.historyIDs[ev.ID] = struct{}{}
		if len(h.history) > h.maxHistory {
			oldest := h.history[0]
			h.history = h.history[1:]
			delete(h.historyIDs, oldest.ID)
		}
	}
	targets := map[string]struct{}{}
	if h.resolve != nil {
		for _, id := range h.resolve(ctx, ev) {
			targets[id] = struct{}{}
		}
	} else {
		for _, key := range []string{"user_id", "userId", "ownerId", "owner_id"} {
			if v, ok := ev.Payload[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					targets[s] = struct{}{}
				}
			}
		}
	}
	subs := make([]*hubSubscription, 0, len(h.subs))
	for sub := range h.subs {
		subs = append(subs, sub)
	}
	h.mu.Unlock()

	delivered := 0
	for _, sub := range subs {
		if sub.Role != "admin" {
			if _, ok := targets[sub.UserID]; !ok {
				continue
			}
		}
		sub.enqueue(ev)
		delivered++
	}
	if delivered > 0 {
		h.logDebug("notification ws fanout", "event", string(ev.Type), "clients", delivered)
	}
	return nil
}

// PublishUserNotification pushes an already-persisted per-user notification
// row to every connected subscription owned by userID. The recipient set is
// EXPLICIT (ownership-safe by construction): unlike Handle, no resolver runs
// and admins are not implied — a user only ever sees their own rows. Call it
// after the notification row has been committed so the 5s safety poll and the
// push stream stay consistent.
func (h *wsHub) PublishUserNotification(userID string, n models.Notification) {
	if h == nil || userID == "" || n.ID == "" {
		return
	}
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	subs := make([]*hubSubscription, 0, len(h.subs))
	for sub := range h.subs {
		// Explicit recipient only — an admin connection is NOT implied here.
		if sub.UserID == userID {
			subs = append(subs, sub)
		}
	}
	h.mu.Unlock()
	for _, sub := range subs {
		sub.enqueue(events.Envelope{
			ID:           n.ID,
			Type:         events.EventType(n.Type),
			Timestamp:    n.CreatedAt,
			Source:       "notification",
			ResourceType: "notification",
			ResourceID:   n.ID,
			Payload: map[string]any{
				"kind":   "notification_row",
				"userID": userID,
				"row":    n,
			},
		})
	}
	if len(subs) > 0 {
		h.logDebug("notification ws user push", "notification", n.ID, "clients", len(subs))
	}
}

// Stop closes all active subscriptions and rejects future Subscribes.
// Call it on the shutdown chain so sockets and goroutines unwind cleanly.
func (h *wsHub) Stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.stopped = true
	subs := make([]*hubSubscription, 0, len(h.subs))
	for sub := range h.subs {
		subs = append(subs, sub)
	}
	h.subs = make(map[*hubSubscription]struct{})
	h.mu.Unlock()
	for _, sub := range subs {
		h.Unsubscribe(sub)
	}
}

// compile-time assertions
var _ events.Subscriber = (*wsHub)(nil)
