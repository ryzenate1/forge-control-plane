package auditlog

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"gamepanel/forge/internal/models"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type AuditEvent struct {
	ID           string         `json:"id"`
	UserID       string         `json:"userId"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource"`
	ResourceID   string         `json:"resourceId"`
	Details      map[string]any `json:"metadata,omitempty"`
	IP           string         `json:"ip"`
	UserAgent    string         `json:"userAgent"`
	Timestamp    time.Time      `json:"createdAt"`
}

type AuditFilter struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Since        time.Time
	Until        time.Time
	Limit        int
}

type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
}

type InMemoryAuditLogger struct {
	mu       sync.RWMutex
	events   []AuditEvent
	capacity int
	head     int
	count    int
}

func NewInMemoryAuditLogger(capacity int) *InMemoryAuditLogger {
	if capacity <= 0 {
		capacity = 10000
	}
	return &InMemoryAuditLogger{
		events:   make([]AuditEvent, capacity),
		capacity: capacity,
	}
}

func (l *InMemoryAuditLogger) Log(_ context.Context, event AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	l.events[l.head] = event
	l.head = (l.head + 1) % l.capacity
	if l.count < l.capacity {
		l.count++
	}
	return nil
}

func (l *InMemoryAuditLogger) Query(_ context.Context, filter AuditFilter) ([]AuditEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []AuditEvent
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	for i := 0; i < l.count; i++ {
		idx := (l.head - 1 - i + l.capacity) % l.capacity
		event := l.events[idx]

		if filter.UserID != "" && event.UserID != filter.UserID {
			continue
		}
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.ResourceType != "" && event.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && event.ResourceID != filter.ResourceID {
			continue
		}
		if !filter.Since.IsZero() && event.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && event.Timestamp.After(filter.Until) {
			continue
		}

		results = append(results, event)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

type DBAuditLogger struct {
	store *store.Store
}

func NewDBAuditLogger(s *store.Store) *DBAuditLogger {
	return &DBAuditLogger{store: s}
}

func (l *DBAuditLogger) Log(ctx context.Context, event AuditEvent) error {
	rec := &models.AuditLog{
		UserID:       event.UserID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		IPAddress:    event.IP,
		UserAgent:    event.UserAgent,
	}
	if event.Timestamp.IsZero() {
		rec.CreatedAt = time.Now()
	} else {
		rec.CreatedAt = event.Timestamp
	}
	if event.Details != nil {
		rec.Details = models.JSONMap(event.Details)
	}
	if event.ID != "" {
		rec.ID = event.ID
	} else {
		rec.ID = uuid.NewString()
	}
	return l.store.CreateAuditLog(ctx, rec)
}

func (l *DBAuditLogger) Query(ctx context.Context, filter AuditFilter) ([]AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	// The admin audit trail reads the live audit_events table: it is the table
	// every production writer appends to via Store.AppendAudit. The legacy
	// audit_logs table has no production writers, so querying it would report
	// an always-empty trail. audit_events carries no IP or user-agent columns,
	// so those fields stay empty rather than fabricated.
	rows, err := l.store.QueryAuditEvents(ctx, store.AuditEventsFilter{
		UserID:       filter.UserID,
		Action:       filter.Action,
		ResourceType: filter.ResourceType,
		ResourceID:   filter.ResourceID,
		Since:        filter.Since,
		Until:        filter.Until,
		Limit:        limit,
	})
	if err != nil {
		return nil, err
	}
	events := make([]AuditEvent, 0, len(rows))
	for _, r := range rows {
		ev := AuditEvent{
			ID:           r.ID,
			Action:       r.Action,
			ResourceType: r.TargetType,
			Timestamp:    r.CreatedAt,
		}
		if r.ActorID != nil {
			ev.UserID = *r.ActorID
		}
		if r.TargetID != nil {
			ev.ResourceID = *r.TargetID
		}
		if r.Metadata != "" && r.Metadata != "{}" {
			var details map[string]any
			if jerr := json.Unmarshal([]byte(r.Metadata), &details); jerr == nil {
				ev.Details = details
			}
		}
		events = append(events, ev)
	}
	return events, nil
}

type AuditLogHandler struct {
	logger AuditLogger
}

func NewAuditLogHandler(logger AuditLogger) *AuditLogHandler {
	return &AuditLogHandler{logger: logger}
}

func (h *AuditLogHandler) HandleQuery(c *fiber.Ctx) error {
	filter := AuditFilter{
		UserID:       c.Query("user_id"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		ResourceID:   c.Query("resource_id"),
	}

	if since := c.Query("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = t
		}
	}
	if until := c.Query("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filter.Until = t
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		var limit int
		for _, ch := range limitStr {
			if ch >= '0' && ch <= '9' {
				limit = limit*10 + int(ch-'0')
			}
		}
		if limit > 0 {
			filter.Limit = limit
		}
	}

	events, err := h.logger.Query(c.Context(), filter)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if events == nil {
		events = []AuditEvent{}
	}
	return c.JSON(fiber.Map{
		"data": events,
		"meta": fiber.Map{
			"total": len(events),
		},
	})
}
