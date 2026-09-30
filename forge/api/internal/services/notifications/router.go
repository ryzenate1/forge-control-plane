package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

// Sentinel errors mapped to HTTP status codes by the notification handlers.
var (
	// ErrChannelNotFound is returned when a channel id does not exist.
	ErrChannelNotFound = errors.New("notification channel not found")
	// ErrForbidden is returned when a user touches a channel they do not own.
	ErrForbidden = errors.New("you do not have permission to manage this notification channel")
	// ErrSubscriptionNotFound is returned when a subscription id does not exist.
	ErrSubscriptionNotFound = errors.New("notification subscription not found")
)

// ChannelStore is the narrow persistence seam the Router needs. *store.Store
// satisfies it; keeping it local avoids depending on the whole store surface.
type ChannelStore interface {
	GetNotificationChannel(ctx context.Context, id string) (store.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context) ([]store.NotificationChannel, error)
	CreateNotificationChannel(ctx context.Context, req store.CreateNotificationChannelRequest) (store.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, id string, req store.UpdateNotificationChannelRequest) (store.NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, id string) error

	ListNotificationEventSubscriptions(ctx context.Context, channelID string) ([]store.NotificationEventSubscription, error)
	GetNotificationEventSubscription(ctx context.Context, channelID, eventType string) (store.NotificationEventSubscription, error)
	CreateNotificationEventSubscription(ctx context.Context, channelID, eventType, templateText string) (store.NotificationEventSubscription, error)
	DeleteNotificationEventSubscription(ctx context.Context, id string) error

	CreateNotificationLog(ctx context.Context, channelID, eventType, status, errorMsg string) (store.NotificationLog, error)
	UpdateNotificationSubscriptionDelivery(ctx context.Context, channelID, eventType, status string) error
}

// Router is the notifications engine: it owns channel CRUD scoping, event
// subscription matching, template rendering and concurrent fan-out. It
// implements events.Subscriber so the control-plane event bus can feed it
// directly (webhook-handler-style consumer), and exposes DispatchEvent for
// services that want to emit catalog events (ssl.expiring, preview.created)
// without going through the bus.
type Router struct {
	store  ChannelStore
	mail   MailEnqueuer
	http   *WebhookService
	logger *slog.Logger
	slots  chan struct{}

	tmplMu sync.Mutex
	tmpls  map[string]*template.Template

	// dispatchMu guards recent, a short window keyed on (canonical event,
	// resource) used to collapse the same fact arriving over more than one
	// transport (the events bus and the webhook outbox both report power
	// state changes, which would otherwise double-send).
	dispatchMu sync.Mutex
	recent     map[string]time.Time
}

// dispatchDedupeWindow bounds how aggressively identical events are collapsed.
const dispatchDedupeWindow = 5 * time.Second

// maxDedupeEntries bounds the dedupe cache; overruns evict the oldest keys.
const maxDedupeEntries = 2048

// NewRouter wires the engine against the shared store and durable mail outbox.
func NewRouter(s ChannelStore, mail MailEnqueuer, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		store:  s,
		mail:   mail,
		http:   NewWebhookService(),
		logger: logger,
		slots:  make(chan struct{}, 32),
		tmpls:  map[string]*template.Template{},
		recent: map[string]time.Time{},
	}
}

// RegisterWith subscribes the router to every event name the catalog knows how
// to render. Subscribing per name (rather than wildcard) keeps high-frequency
// control-plane events from costing a channel/subscription lookup each.
func (r *Router) RegisterWith(registry *events.Registry) {
	if registry == nil {
		return
	}
	subscribed := map[string]struct{}{}
	for _, entry := range NotificationCatalog {
		for _, name := range entry.InternalTypes {
			if _, done := subscribed[name]; done {
				continue
			}
			subscribed[name] = struct{}{}
			registry.Subscribe(events.EventType(name), events.SubscriberWithRecovery(events.Subscriber(r)))
		}
	}
}

// Handle implements events.Subscriber. An empty ResourceID is passed through
// so dispatch() can fall back to the payload's identity keys (the correlation
// id is per-envelope and would defeat duplicate detection).
func (r *Router) Handle(ctx context.Context, ev events.Envelope) error {
	return r.dispatch(ctx, string(ev.Type), ev.ResourceID, ev.ResourceType, ev.Payload)
}

// DispatchEvent fans a catalog event out to every subscribed, enabled
// channel. Payload keys may carry the reserved "title", "summary",
// "resource_id" and "resource_type" overrides. Unknown event names still
// dispatch (they resolve to a synthetic custom descriptor) so services can
// emit integration events without a catalog change first.
func (r *Router) DispatchEvent(ctx context.Context, eventType string, payload map[string]any) error {
	return r.dispatch(ctx, eventType, "", "", payload)
}

// delivery is one (channel, subscription) fan-out unit.
type delivery struct {
	channel      store.NotificationChannel
	notifier     Notifier
	subscription *store.NotificationEventSubscription
}

// resourceIDPayloadKeys are the identity keys producers actually use: the
// events bus fills Envelope.ResourceID, while webhook-outbox payloads carry
// "subject_id" and per-domain ids.
var resourceIDPayloadKeys = []string{"resource_id", "subject_id", "server_id", "node_id", "stackId", "app_id", "deployment_id", "id"}

func firstPayloadString(payload map[string]any, keys []string) string {
	for _, key := range keys {
		if value, ok := stringifyTemplateValue(payload[key]); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// claim reports whether this (event, resource) pair should be delivered now,
// collapsing duplicates that arrive over more than one transport within
// dispatchDedupeWindow.
func (r *Router) claim(key string) bool {
	now := time.Now()
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	if seen, ok := r.recent[key]; ok && now.Sub(seen) < dispatchDedupeWindow {
		return false
	}
	if len(r.recent) >= maxDedupeEntries {
		for k, seen := range r.recent {
			if now.Sub(seen) >= dispatchDedupeWindow {
				delete(r.recent, k)
			}
		}
		if len(r.recent) >= maxDedupeEntries {
			// Sustained burst: drop the window rather than grow unbounded.
			r.recent = map[string]time.Time{}
		}
	}
	r.recent[key] = now
	return true
}

func (r *Router) dispatch(ctx context.Context, rawEventType, resourceID, resourceType string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	descriptor := ResolveEvent(rawEventType)
	matchNames := descriptor.EventNamesFor()
	matchNames = append(matchNames, descriptor.InternalTypes...)
	if rawEventType != descriptor.Type {
		// Exact-match for custom (non-catalog) events that have no descriptor names.
		matchNames = append(matchNames, rawEventType)
	}
	if resourceID == "" {
		resourceID = firstPayloadString(payload, resourceIDPayloadKeys)
	}
	if resourceType == "" {
		resourceType = firstPayloadString(payload, []string{"resource_type", "subject_type"})
	}
	if resourceID != "" {
		// Power-state changes are reported both on the events bus
		// (ServerStarted) and through the webhook outbox (server:started);
		// deliver the first and drop the rest of the window.
		if !r.claim(descriptor.Type + "\x00" + resourceID) {
			return nil
		}
	}

	channels, err := r.store.ListNotificationChannels(ctx)
	if err != nil {
		return fmt.Errorf("list notification channels: %w", err)
	}
	subscriptions, err := r.store.ListNotificationEventSubscriptions(ctx, "")
	if err != nil {
		return fmt.Errorf("list notification subscriptions: %w", err)
	}
	channelByID := make(map[string]store.NotificationChannel, len(channels))
	for _, ch := range channels {
		channelByID[ch.ID] = ch
	}

	// Match in canonical-name order first so a channel subscribed to both the
	// canonical and a legacy alias is only delivered once.
	deliveries := []delivery{}
	seen := map[string]struct{}{}
	for _, name := range matchNames {
		for _, sub := range subscriptions {
			if !subscriptionMatches(sub.EventType, name) {
				continue
			}
			ch, ok := channelByID[sub.ChannelID]
			if !ok || !ch.Enabled {
				continue
			}
			if _, done := seen[ch.ID]; done {
				continue
			}
			notifier, err := r.notifierFor(ch)
			if err != nil {
				r.logger.Error("notification channel misconfigured", "channel", ch.ID, "event", name, "error", err)
				if logErr := r.logDelivery(ctx, ch.ID, sub.EventType, "failed", err.Error(), &sub); logErr != nil {
					r.logger.Error("record notification log", "error", logErr)
				}
				continue
			}
			seen[ch.ID] = struct{}{}
			subCopy := sub
			deliveries = append(deliveries, delivery{channel: ch, notifier: notifier, subscription: &subCopy})
		}
	}
	if len(deliveries) == 0 {
		return nil
	}

	data := r.renderData(descriptor, rawEventType, resourceID, resourceType, payload)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, d := range deliveries {
		notification, err := r.render(d, data, descriptor)
		if err != nil {
			// Honest, per-channel failure: a broken custom template must not
			// silence the other subscribers.
			r.logger.Error("render notification", "channel", d.channel.ID, "event", rawEventType, "error", err)
			if logErr := r.logDelivery(ctx, d.channel.ID, rawEventType, "failed", err.Error(), d.subscription); logErr != nil {
				r.logger.Error("record notification log", "error", logErr)
			}
			continue
		}

		wg.Add(1)
		select {
		case r.slots <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			return ctx.Err()
		}
		go func(d delivery, notification Notification) {
			defer func() { <-r.slots; wg.Done() }()
			sendErr := r.deliver(ctx, d, notification)
			mu.Lock()
			if sendErr != nil {
				errs = append(errs, fmt.Errorf("channel %s: %w", d.channel.Name, sendErr))
			}
			mu.Unlock()
		}(d, notification)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// deliver sends one notification and records the audit trail (notification_logs
// + subscription delivery status) like the legacy service does.
func (r *Router) deliver(ctx context.Context, d delivery, notification Notification) error {
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	err := d.notifier.Send(sendCtx, notification)
	status := "delivered"
	errorMsg := ""
	if err != nil {
		status = "failed"
		errorMsg = err.Error()
	}
	if logErr := r.logDelivery(sendCtx, d.channel.ID, notification.Event, status, errorMsg, d.subscription); logErr != nil {
		r.logger.Error("record notification delivery", "channel", d.channel.ID, "event", notification.Event, "error", logErr)
	}
	return err
}

func (r *Router) logDelivery(ctx context.Context, channelID, eventType, status, errorMsg string, sub *store.NotificationEventSubscription) error {
	if _, err := r.store.CreateNotificationLog(ctx, channelID, eventType, status, errorMsg); err != nil {
		return err
	}
	if sub == nil {
		return nil
	}
	return r.store.UpdateNotificationSubscriptionDelivery(ctx, channelID, sub.EventType, status)
}

// subscriptionMatches supports exact matches, "*" wildcards and "prefix:*"
// category globs ("deploy:*" matches "deploy.success").
func subscriptionMatches(subscribed, eventType string) bool {
	if subscribed == eventType {
		return true
	}
	if subscribed == "*" {
		return true
	}
	if prefix, glob := strings.CutSuffix(subscribed, ":*"); glob {
		return strings.HasPrefix(eventType, prefix+".")
	}
	return false
}

// --- Rendering ------------------------------------------------------------

var resourcePayloadKeys = []string{"name", "subject_name", "server_name", "app_name", "stack", "stack_name", "display_name", "uuid", "domain", "url", "id"}

func (r *Router) renderData(descriptor EventDescriptor, rawEventType, resourceID, resourceType string, payload map[string]any) renderData {
	if id := firstPayloadString(payload, resourceIDPayloadKeys); id != "" {
		resourceID = id
	}
	if rt, ok := payload["resource_type"].(string); ok && strings.TrimSpace(rt) != "" {
		resourceType = strings.TrimSpace(rt)
	} else if rt := firstPayloadString(payload, []string{"subject_type"}); rt != "" {
		resourceType = rt
	}
	if resourceType == "" {
		resourceType = "resource"
	}
	resource := resourceID
	for _, key := range resourcePayloadKeys {
		if value, ok := stringifyTemplateValue(payload[key]); ok && value != "" {
			resource = value
			break
		}
	}
	title := descriptor.Name
	if custom, ok := payload["title"].(string); ok && strings.TrimSpace(custom) != "" {
		title = strings.TrimSpace(custom)
	}
	summary := strings.TrimSpace(descriptor.Description)
	if custom, ok := payload["summary"].(string); ok && strings.TrimSpace(custom) != "" {
		summary = strings.TrimSpace(custom)
	}
	return renderData{
		Event:        descriptor.Type,
		Title:        title,
		Summary:      summary,
		Severity:     descriptor.Severity,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Resource:     resource,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Payload:      payload,
	}
}

// compileTemplate parses (and caches) a user-supplied template so repeated
// subscriptions to the same channel template don't re-parse per delivery.
func (r *Router) compileTemplate(text string) (*template.Template, error) {
	r.tmplMu.Lock()
	defer r.tmplMu.Unlock()
	if cached, ok := r.tmpls[text]; ok {
		return cached, nil
	}
	parsed, err := template.New("notification").Funcs(template.FuncMap(templateFuncs)).Parse(text)
	if err != nil {
		return nil, err
	}
	if len(r.tmpls) < 256 {
		r.tmpls[text] = parsed
	}
	return parsed, nil
}

func (r *Router) render(d delivery, data renderData, descriptor EventDescriptor) (Notification, error) {
	body, err := r.renderTemplate(d.subscription.Template, data, defaultTemplate)
	if err != nil {
		// A broken custom template falls back to the catalog default; only if
		// the default itself fails (impossible in practice) do we skip.
		body, err = r.renderTemplate(descriptor.DefaultTmpl, data, defaultTemplate)
		if err != nil {
			return Notification{}, err
		}
	}
	return Notification{
		Event:     data.Event,
		Title:     data.Title,
		Body:      strings.TrimSpace(body),
		Severity:  descriptor.Severity,
		Timestamp: time.Now().UTC(),
		Payload:   data.Payload,
	}, nil
}

func (r *Router) renderTemplate(text string, data renderData, fallback string) (string, error) {
	if strings.TrimSpace(text) == "" {
		text = fallback
	}
	parsed, err := r.compileTemplate(text)
	if err != nil {
		return "", fmt.Errorf("invalid notification template: %w", err)
	}
	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render notification template: %w", err)
	}
	return out.String(), nil
}

// --- Scoped CRUD -----------------------------------------------------------

// Viewer identifies the caller for channel/subscription ownership checks.
type Viewer struct {
	UserID  string
	IsAdmin bool
}

func (v Viewer) canManage(ch store.NotificationChannel) bool {
	if v.IsAdmin {
		return true
	}
	return ch.UserID != nil && *ch.UserID == v.UserID
}

// ListChannels returns global channels (admins manage these; everyone sees
// them so subscriptions can be created) plus the viewer's own channels.
func (r *Router) ListChannels(ctx context.Context, viewer Viewer) ([]store.NotificationChannel, error) {
	channels, err := r.store.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := []store.NotificationChannel{}
	for _, ch := range channels {
		if ch.UserID == nil || (ch.UserID != nil && *ch.UserID == viewer.UserID) || viewer.IsAdmin {
			result = append(result, ch)
		}
	}
	return result, nil
}

// CreateChannel validates the configuration through the notifier factory,
// then stamps ownership: regular users own their channels; admins may create
// global (both ids nil) or org-wide channels.
func (r *Router) CreateChannel(ctx context.Context, viewer Viewer, req store.CreateNotificationChannelRequest) (store.NotificationChannel, error) {
	if strings.TrimSpace(req.Name) == "" {
		return store.NotificationChannel{}, fmt.Errorf("channel name is required")
	}
	if err := ValidateChannelConfig(req.Type, req.Config); err != nil {
		return store.NotificationChannel{}, err
	}
	if !viewer.IsAdmin {
		req.UserID = &viewer.UserID
		req.OrgID = nil
	}
	return r.store.CreateNotificationChannel(ctx, req)
}

// UpdateChannel patches name/config/enabled after an ownership check.
func (r *Router) UpdateChannel(ctx context.Context, viewer Viewer, id string, req store.UpdateNotificationChannelRequest) (store.NotificationChannel, error) {
	existing, err := r.store.GetNotificationChannel(ctx, id)
	if err != nil {
		return store.NotificationChannel{}, translateStoreError(err)
	}
	if !viewer.canManage(existing) {
		return store.NotificationChannel{}, ErrForbidden
	}
	if req.Config != nil {
		if err := ValidateChannelConfig(existing.Type, *req.Config); err != nil {
			return store.NotificationChannel{}, err
		}
	}
	updated, err := r.store.UpdateNotificationChannel(ctx, id, req)
	if err != nil {
		return store.NotificationChannel{}, translateStoreError(err)
	}
	return updated, nil
}

// DeleteChannel removes a channel (subscriptions cascade via FK) after an
// ownership check.
func (r *Router) DeleteChannel(ctx context.Context, viewer Viewer, id string) error {
	existing, err := r.store.GetNotificationChannel(ctx, id)
	if err != nil {
		return translateStoreError(err)
	}
	if !viewer.canManage(existing) {
		return ErrForbidden
	}
	return r.store.DeleteNotificationChannel(ctx, id)
}

// Subscribe attaches an event type to a channel. eventType is normalized to
// the catalog's canonical name when a legacy alias is supplied, so duplicate
// spellings never coexist on one channel.
func (r *Router) Subscribe(ctx context.Context, viewer Viewer, channelID, eventType, templateText string) (store.NotificationEventSubscription, error) {
	ch, err := r.store.GetNotificationChannel(ctx, channelID)
	if err != nil {
		return store.NotificationEventSubscription{}, translateStoreError(err)
	}
	if !viewer.canManage(ch) {
		return store.NotificationEventSubscription{}, ErrForbidden
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return store.NotificationEventSubscription{}, fmt.Errorf("event type is required")
	}
	if descriptor, ok := LookupEvent(strings.ToLower(eventType)); ok {
		eventType = descriptor.Type
	} else if !strings.ContainsAny(eventType, "*") && !IsKnownEventName(eventType) {
		return store.NotificationEventSubscription{}, fmt.Errorf("unknown event type %q (see GET /api/v1/notifications/events)", eventType)
	}
	if strings.TrimSpace(templateText) != "" {
		if _, err := r.compileTemplate(templateText); err != nil {
			return store.NotificationEventSubscription{}, err
		}
	}
	return r.store.CreateNotificationEventSubscription(ctx, channelID, eventType, templateText)
}

// ListSubscriptions returns every subscription on channels the viewer can see.
func (r *Router) ListSubscriptions(ctx context.Context, viewer Viewer) ([]store.NotificationEventSubscription, error) {
	channels, err := r.ListChannels(ctx, viewer)
	if err != nil {
		return nil, err
	}
	visible := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		visible[ch.ID] = struct{}{}
	}
	subs, err := r.store.ListNotificationEventSubscriptions(ctx, "")
	if err != nil {
		return nil, err
	}
	result := []store.NotificationEventSubscription{}
	for _, sub := range subs {
		if _, ok := visible[sub.ChannelID]; ok {
			result = append(result, sub)
		}
	}
	return result, nil
}

// Unsubscribe removes a subscription by id after an ownership check on its
// channel.
func (r *Router) Unsubscribe(ctx context.Context, viewer Viewer, id string) error {
	sub, err := r.findSubscription(ctx, id)
	if err != nil {
		return err
	}
	ch, err := r.store.GetNotificationChannel(ctx, sub.ChannelID)
	if err != nil {
		return translateStoreError(err)
	}
	if !viewer.canManage(ch) {
		return ErrForbidden
	}
	return r.store.DeleteNotificationEventSubscription(ctx, id)
}

func (r *Router) findSubscription(ctx context.Context, id string) (store.NotificationEventSubscription, error) {
	subs, err := r.store.ListNotificationEventSubscriptions(ctx, "")
	if err != nil {
		return store.NotificationEventSubscription{}, err
	}
	for _, sub := range subs {
		if sub.ID == id {
			return sub, nil
		}
	}
	return store.NotificationEventSubscription{}, ErrSubscriptionNotFound
}

// notifierFor builds the channel's notifier against the router's shared
// transports (one http.Client, the durable mail outbox).
func (r *Router) notifierFor(ch store.NotificationChannel) (Notifier, error) {
	return newNotifierWithDeps(ch, notifierDeps{http: r.http, mail: r.mail})
}

// TestChannel sends a fixed test notification through the channel after an
// ownership check, surfacing the provider's response honestly.
func (r *Router) TestChannel(ctx context.Context, viewer Viewer, id string) error {
	ch, err := r.store.GetNotificationChannel(ctx, id)
	if err != nil {
		return translateStoreError(err)
	}
	if !viewer.canManage(ch) {
		return ErrForbidden
	}
	notifier, err := r.notifierFor(ch)
	if err != nil {
		return err
	}
	notification := Notification{
		Event:     "notification.test",
		Title:     "Forge test notification",
		Body:      fmt.Sprintf("This is a test notification for the %q channel. If you can see this, delivery works.", ch.Name),
		Severity:  LevelInfo,
		Timestamp: time.Now().UTC(),
	}
	return notifier.Send(ctx, notification)
}

// Catalog exposes the event catalog for the GET /notifications/events route.
func (r *Router) Catalog() []EventDescriptor {
	return NotificationCatalog
}

// translateStoreError converts sql.ErrNoRows into ErrChannelNotFound so the
// HTTP layer can answer 404 instead of 500.
func translateStoreError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrChannelNotFound
	}
	return err
}

// compile-time assertions
var (
	_ events.Subscriber = (*Router)(nil)
	_ Notifier          = (*DiscordNotifier)(nil)
	_ Notifier          = (*SlackNotifier)(nil)
	_ Notifier          = (*TelegramNotifier)(nil)
	_ Notifier          = (*EmailNotifier)(nil)
	_ Notifier          = (*WebhookNotifier)(nil)
)
