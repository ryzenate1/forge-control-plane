package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// Real-time Docker events feed.
//
// Beacon tails its local Docker event stream and batches container lifecycle
// events to POST /api/remote/docker/events (node HMAC auth — the same scheme as
// the heartbeat). The panel stores them, fans the notable ones into the
// notifications engine as `docker.event.<type>`, and serves a filtered,
// paginated timeline on GET /api/v1/admin/docker/events.
//
// Both surfaces are mounted from the phase-hook registrar, so the feature adds
// no routes to NewServer: the admin timeline hangs off the authenticated
// /api/v1 group, and the node-facing ingest is attached under /api/remote with
// the same guard chain NewServer builds for that prefix (API IP allowlist,
// mTLS, node bearer+HMAC).

const dockerEventsPriority = 230

const (
	// dockerEventIngestLimit caps one batch. Beacon flushes at 50 events per
	// request; the headroom absorbs a manual replay without letting a client
	// make the API build a megabyte-scale VALUES list. It matches the store's
	// own per-insert ceiling.
	dockerEventIngestLimit = 500
	// dockerEventRetention is how long the timeline remembers events. The feed
	// is an operational window, not an audit log — durable history belongs to
	// the activity tables, which are pruned on their own schedule.
	dockerEventRetention = 30 * 24 * time.Hour
	// The delay keeps the first sweep off the boot path; the interval only has
	// to be fine relative to a 30-day window.
	dockerEventRetentionDelay    = 10 * time.Minute
	dockerEventRetentionInterval = time.Hour
)

// dockerEventsNotificationTypes are the lifecycle actions fanned out to
// subscribers. Everything is still stored for the timeline, but a rolling
// restart of a hundred workloads must not become a hundred start/stop
// notifications — only the actions an operator would want poked about.
var dockerEventsNotificationTypes = map[string]bool{
	"die":      true,
	"kill":     true,
	"oom":      true,
	"recreate": true,
	"destroy":  true,
}

var dockerEventsRetentionOnce sync.Once

func init() {
	RegisterPhaseRegistrar("docker-events", dockerEventsPriority, registerDockerEventRoutes)
}

func registerDockerEventRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg == nil || cfg.Store == nil || protected == nil {
		return nil
	}
	// Handler factories in this package take Config by value, which is how every
	// route receives it; the registrar contract hands over a pointer.
	conf := *cfg

	admin := protected.Group("/admin/docker", requireRole("admin"))
	admin.Get("/events", listAdminDockerEvents(conf))

	if err := registerDockerEventIngest(v1, conf); err != nil {
		return err
	}

	startDockerEventRetention(conf)
	return nil
}

// registerDockerEventIngest mounts the node-facing batch endpoint under the
// /api/remote prefix. NewServer builds that group before the phase hooks run, so
// a registrar cannot reach the *fiber.Group itself; instead the identical guard
// chain is assembled here from the same Config and attached to the app through
// the v1 router. Fiber registers each group's routes with their own middleware
// list, so a separate /api/remote/docker group neither shadows nor reorders the
// existing /api/remote/servers/... routes.
func registerDockerEventIngest(v1 fiber.Router, cfg Config) error {
	if v1 == nil || v1.App() == nil {
		return errors.New("docker event ingest: no application router")
	}
	if cfg.NodeRegistry == nil {
		// Without the registry a bearer token cannot be resolved to a node at all,
		// and remoteNodeMiddleware would panic on the first report. The timeline is
		// still useful for whatever is already stored, so this is a warning and not
		// a registrar failure.
		logDockerEventIngestSkipped(cfg)
		return nil
	}

	guards := make([]fiber.Handler, 0, 3)
	// The IP guard is only instantiated when it has something to say:
	// APIIPAccessConfig logs a "not configured" warning and NewServer already
	// emitted exactly one for the real /api/remote group, so calling it again
	// unconditionally would duplicate the message on every boot.
	if strings.TrimSpace(os.Getenv("API_IP_ALLOW")) != "" || strings.TrimSpace(os.Getenv("API_IP_DENY")) != "" {
		guards = append(guards, IPAccessControl(APIIPAccessConfig(cfg)))
	}
	guards = append(guards, MTLSAuthMiddleware(dockerEventsMTLSConfig(cfg)))
	guards = append(guards, remoteNodeMiddleware(cfg, cfg.NodeRegistry))

	remote := v1.App().Group("/api/remote/docker", guards...)
	remote.Post("/events", ingestDockerEvents(cfg))
	return nil
}

// dockerEventsMTLSConfig mirrors the mTLS settings NewServer applies to
// /api/remote, so a deployment that requires client certificates on the
// node-facing prefix cannot have its event reports arrive through a side door.
func dockerEventsMTLSConfig(cfg Config) MTLSAuthConfig {
	mtlsCfg := MTLSAuthConfig{
		Enabled:    cfg.MTLSEnabled,
		CACertPath: cfg.MTLSCACertPath,
		CertPath:   cfg.MTLSCertPath,
		KeyPath:    cfg.MTLSKeyPath,
		DevBypass:  cfg.MTLSDevBypass,
	}
	if cfg.Store != nil {
		mtlsCfg.RevocationLookup = cfg.Store.IsMTLSCertificateRevoked
	}
	return mtlsCfg
}

func logDockerEventIngestSkipped(cfg Config) {
	const message = "docker event ingest not mounted: node registry service is unavailable"
	if cfg.Logger != nil {
		cfg.Logger.Warn(message)
		return
	}
	slog.Warn(message)
}

// dockerEventPayload is one event as Beacon reports it, mirroring
// events.Message: {type, containerID, containerName, image, actorAttributes,
// timestamp}. encoding/json matches names case-insensitively, so a node sending
// containerId rather than containerID still lands.
type dockerEventPayload struct {
	Type            string         `json:"type"`
	ContainerID     string         `json:"containerID"`
	ContainerName   string         `json:"containerName"`
	Image           string         `json:"image"`
	ActorAttributes map[string]any `json:"actorAttributes"`
	Timestamp       string         `json:"timestamp"`
}

func ingestDockerEvents(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}

		payloads, err := parseDockerEventBatch(c.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if len(payloads) == 0 {
			// An empty batch is a legitimate flush tick with nothing new; answer
			// 204 instead of pretending rows were written.
			return c.SendStatus(fiber.StatusNoContent)
		}

		// Node identity comes from the authenticated token, never from the body:
		// a node cannot attribute its events to another host.
		now := time.Now().UTC()
		events := make([]store.DockerEvent, 0, len(payloads))
		for index, payload := range payloads {
			event, err := dockerEventFromPayload(node, now, index, payload)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
			events = append(events, event)
		}

		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.InsertDockerEvents(ctx, events); err != nil {
			return respondInternalError(c, err)
		}

		// Notification fan-out is deliberately off the response path: a webhook
		// channel that has stopped answering must not turn an already-stored event
		// into a 502 for the node, which would only make Beacon re-send a batch the
		// dedupe index has now seen once.
		go dispatchDockerEventNotifications(cfg, node, events)

		return c.JSON(fiber.Map{"accepted": len(events)})
	}
}

// parseDockerEventBatch accepts either {"events":[...]} or a bare JSON array.
// The envelope is what Beacon sends; the bare form exists so an operator can
// replay a captured batch with a one-line curl.
func parseDockerEventBatch(body []byte) ([]dockerEventPayload, error) {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, nil
	}
	var payloads []dockerEventPayload
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &payloads); err != nil {
			return nil, fmt.Errorf("invalid request body: %s", err.Error())
		}
	} else {
		var envelope struct {
			Events []dockerEventPayload `json:"events"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, fmt.Errorf("invalid request body: %s", err.Error())
		}
		payloads = envelope.Events
	}
	if len(payloads) > dockerEventIngestLimit {
		return nil, fmt.Errorf("batch contains %d events, the limit is %d per request", len(payloads), dockerEventIngestLimit)
	}
	return payloads, nil
}

func dockerEventFromPayload(node store.Node, now time.Time, index int, payload dockerEventPayload) (store.DockerEvent, error) {
	eventType := strings.ToLower(strings.TrimSpace(payload.Type))
	if eventType == "" {
		return store.DockerEvent{}, fmt.Errorf("event %d: type is required", index)
	}
	if len(eventType) > 32 {
		return store.DockerEvent{}, fmt.Errorf("event %d: type %q exceeds 32 characters", index, eventType)
	}

	containerID := truncateField(payload.ContainerID, 64)
	if containerID == "" {
		// The dedupe index is keyed on the container id, so a node that sent only
		// the actor "id" attribute still gets a usable identity.
		containerID = truncateField(stringifyAttrValue(payload.ActorAttributes["id"]), 64)
	}

	return store.DockerEvent{
		NodeID:        node.ID,
		EventType:     eventType,
		ContainerID:   containerID,
		ContainerName: truncateField(payload.ContainerName, 256),
		Image:         truncateField(payload.Image, 512),
		ActorAttrs:    sanitizeActorAttributes(payload.ActorAttributes),
		Timestamp:     parseDockerEventTime(payload.Timestamp, now),
		IngestedAt:    now,
	}, nil
}

// parseDockerEventTime turns the reported timestamp into a usable instant.
//
// A missing or unparseable value falls back to ingest time rather than failing
// the batch: the event is real and worth keeping, only its age is unknown. A
// timestamp more than a minute ahead of the panel is clamped, because a skewed
// node clock must not push the feed's newest entry ahead of everything else or
// put it outside the retention window.
func parseDockerEventTime(raw string, now time.Time) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return now
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		utc := parsed.UTC()
		if utc.After(now.Add(time.Minute)) {
			return now
		}
		return utc
	}
	// Docker also exposes its event clock as a millisecond epoch; accept a bare
	// integer as seconds or milliseconds.
	if asInt, err := strconv.ParseInt(value, 10, 64); err == nil {
		if asInt > 1e12 {
			return time.UnixMilli(asInt).UTC()
		}
		return time.Unix(asInt, 0).UTC()
	}
	return now
}

// sanitizeActorAttributes bounds untrusted telemetry before it reaches a jsonb
// column: values are stringified and cut to 512 bytes, and a map that still
// cannot fit in 8 KiB is replaced by a marker. Dropping attributes never drops
// the event.
func sanitizeActorAttributes(attrs map[string]any) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	clean := make(map[string]string, len(attrs))
	size := 2
	for key, value := range attrs {
		name := strings.TrimSpace(key)
		if name == "" || len(name) > 64 {
			continue
		}
		text := truncateField(stringifyAttrValue(value), 512)
		clean[name] = text
		size += len(name) + len(text) + 4
	}
	if len(clean) == 0 {
		return nil
	}
	if size > 8*1024 {
		return map[string]string{"truncated": "true"}
	}
	return clean
}

func stringifyAttrValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

// truncateField cuts to a byte budget without splitting a UTF-8 rune, which is
// what keeps a 300-character container name from failing the whole batch on its
// way into a bounded VARCHAR column.
func truncateField(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

// listAdminDockerEvents serves the admin timeline: newest first, filtered by
// node, event type, container substring and time range.
func listAdminDockerEvents(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		filter, err := dockerEventFilterFromRequest(c)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()

		events, err := cfg.Store.ListDockerEvents(ctx, filter)
		if err != nil {
			return respondInternalError(c, err)
		}
		total, err := cfg.Store.CountDockerEvents(ctx, filter)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{
			"events": events,
			"total":  total,
			"limit":  filter.Limit,
			"offset": filter.Offset,
		})
	}
}

func dockerEventFilterFromRequest(c *fiber.Ctx) (store.DockerEventFilter, error) {
	filter := store.DockerEventFilter{}
	if value := strings.TrimSpace(c.Query("node")); value != "" {
		// The column is a uuid, so a malformed value must be refused here rather
		// than turned into a Postgres cast error deep inside a 200-response path.
		parsed, err := uuid.Parse(value)
		if err != nil {
			return filter, errors.New("node must be a node uuid")
		}
		filter.NodeID = parsed.String()
	}
	if value := strings.TrimSpace(c.Query("type")); value != "" {
		filter.EventType = strings.ToLower(value)
	}
	if value := strings.TrimSpace(c.Query("container")); value != "" {
		filter.Container = value
	}
	if value := strings.TrimSpace(c.Query("from")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return filter, errors.New("from must be an RFC3339 timestamp")
		}
		utc := parsed.UTC()
		filter.From = &utc
	}
	if value := strings.TrimSpace(c.Query("to")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return filter, errors.New("to must be an RFC3339 timestamp")
		}
		utc := parsed.UTC()
		filter.To = &utc
	}
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return filter, errors.New("limit must be a positive integer")
		}
		filter.Limit = parsed
	}
	if value := strings.TrimSpace(c.Query("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			return filter, errors.New("offset must be a non-negative integer")
		}
		filter.Offset = parsed
	}
	return filter, nil
}

// dispatchDockerEventNotifications fans stored events into the notifications
// engine. The router resolves an unknown name such as docker.event.oom to a
// synthetic catalog descriptor, so a channel can subscribe to it without a
// catalog change first, and its five-second dedupe window collapses a storm on
// one container into one delivery.
func dispatchDockerEventNotifications(cfg Config, node store.Node, events []store.DockerEvent) {
	if cfg.NotificationRouter == nil || len(events) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(dockerEventsBaseContext(cfg), 2*time.Minute)
	defer cancel()

	nodeName := strings.TrimSpace(node.Name)
	if nodeName == "" {
		nodeName = node.ID
	}

	for _, event := range events {
		if !dockerEventsNotificationTypes[event.EventType] {
			continue
		}
		subject := event.ContainerName
		if subject == "" {
			subject = event.ContainerID
		}
		if subject == "" {
			subject = "unknown container"
		}
		payload := map[string]any{
			"title":          fmt.Sprintf("Container %s: %s", event.EventType, subject),
			"summary":        fmt.Sprintf("%s on node %s at %s", event.EventType, nodeName, event.Timestamp.Format(time.RFC3339)),
			"resource_id":    event.ContainerID,
			"resource_type":  "container",
			"event_type":     event.EventType,
			"node_id":        event.NodeID,
			"node_name":      nodeName,
			"container_id":   event.ContainerID,
			"container_name": event.ContainerName,
			"image":          event.Image,
			"attributes":     event.ActorAttrs,
			"occurred_at":    event.Timestamp.Format(time.RFC3339Nano),
		}
		eventType := "docker.event." + event.EventType
		if err := cfg.NotificationRouter.DispatchEvent(ctx, eventType, payload); err != nil {
			logDockerEventNotifyFailure(cfg, eventType, err)
		}
	}
}

func dockerEventsBaseContext(cfg Config) context.Context {
	if cfg.BackgroundContext != nil {
		return cfg.BackgroundContext
	}
	return context.Background()
}

func logDockerEventNotifyFailure(cfg Config, eventType string, err error) {
	// respondInternalError can only log for a live request; this runs after the
	// response has been sent, so it goes straight to the configured logger.
	if cfg.Logger != nil {
		cfg.Logger.Error("docker event notification dispatch failed",
			slog.String("event", eventType),
			slog.String("error", err.Error()),
		)
		return
	}
	slog.Error("docker event notification dispatch failed",
		slog.String("event", eventType),
		slog.String("error", err.Error()),
	)
}

// startDockerEventRetention runs the 30-day prune. The cleanup service has no
// pluggable retention registry this feature could join without editing it, so
// the sweep lives next to the table it maintains and is started exactly once per
// process (NewServer runs repeatedly in tests).
func startDockerEventRetention(cfg Config) {
	dockerEventsRetentionOnce.Do(func() {
		go runDockerEventRetention(cfg)
	})
}

func runDockerEventRetention(cfg Config) {
	ctx := dockerEventsBaseContext(cfg)

	delay := time.NewTimer(dockerEventRetentionDelay)
	defer delay.Stop()
	select {
	case <-ctx.Done():
		return
	case <-delay.C:
	}

	ticker := time.NewTicker(dockerEventRetentionInterval)
	defer ticker.Stop()
	for {
		pruneCtx, cancel := context.WithTimeout(ctx, time.Minute)
		deleted, err := cfg.Store.PruneDockerEvents(pruneCtx, time.Now().UTC().Add(-dockerEventRetention))
		cancel()
		switch {
		case err != nil:
			if cfg.Logger != nil {
				cfg.Logger.Error("docker event retention sweep failed", slog.String("error", err.Error()))
			}
		case deleted > 0:
			if cfg.Logger != nil {
				cfg.Logger.Info("docker event retention sweep",
					slog.Int64("deleted", deleted),
					slog.String("olderThan", dockerEventRetention.String()),
				)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
