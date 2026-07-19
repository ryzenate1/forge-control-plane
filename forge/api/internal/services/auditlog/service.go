package auditlog

import (
	"context"
	"sync"
	"time"

	"gamepanel/forge/internal/models"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type AuditEvent struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Details      map[string]any `json:"details,omitempty"`
	IP           string         `json:"ip"`
	UserAgent    string         `json:"user_agent"`
	Timestamp    time.Time      `json:"timestamp"`
}

type AuditFilter struct {
	UserID       string
	Action       string
	ResourceType string
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

	var logs []models.AuditLog
	var err error

	if filter.UserID != "" {
		logs, err = l.store.ListAuditLogsByUser(ctx, filter.UserID, limit, 0)
	} else {
		logs, err = l.store.ListRecentAuditLogs(ctx, limit)
	}
	if err != nil {
		return nil, err
	}

	events := make([]AuditEvent, 0, len(logs))
	for _, l := range logs {
		ev := AuditEvent{
			ID:           l.ID,
			UserID:       l.UserID,
			Action:       l.Action,
			ResourceType: l.ResourceType,
			ResourceID:   l.ResourceID,
			IP:           l.IPAddress,
			UserAgent:    l.UserAgent,
			Timestamp:    l.CreatedAt,
		}
		if l.Details != nil {
			ev.Details = map[string]any(l.Details)
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
