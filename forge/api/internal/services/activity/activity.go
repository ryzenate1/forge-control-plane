package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelError    Level = "error"
	LevelCritical Level = "critical"
)

type ActivityEvent struct {
	ID          string          `json:"id"`
	Event       string          `json:"event"`
	Description string          `json:"description,omitempty"`
	ActorID     *string         `json:"actorId,omitempty"`
	ActorEmail  *string         `json:"actorEmail,omitempty"`
	ActorType   string          `json:"actorType"`
	IP          *string         `json:"ip,omitempty"`
	UserAgent   *string         `json:"userAgent,omitempty"`
	SubjectType *string         `json:"subjectType,omitempty"`
	SubjectID   *string         `json:"subjectId,omitempty"`
	SubjectName *string         `json:"subjectName,omitempty"`
	Properties  json.RawMessage `json:"properties"`
	Level       Level           `json:"level"`
	Source      string          `json:"source"`
	Timestamp   time.Time       `json:"timestamp"`
	ExpiresAt   *time.Time      `json:"expiresAt,omitempty"`
}

type ActivityStore interface {
	InsertActivity(ctx context.Context, event *ActivityEvent) error
	QueryActivities(ctx context.Context, filter ActivityFilter) ([]ActivityEvent, error)
	CountActivities(ctx context.Context, filter ActivityFilter) (int, error)
	CleanupActivities(ctx context.Context, before time.Time) (int64, error)
	GetActivityStats(ctx context.Context) (*ActivityStats, error)
}

type ActivityFilter struct {
	ActorID     *string    `json:"actorId,omitempty"`
	SubjectType *string    `json:"subjectType,omitempty"`
	SubjectID   *string    `json:"subjectId,omitempty"`
	Event       *string    `json:"event,omitempty"`
	Level       *Level     `json:"level,omitempty"`
	Source      *string    `json:"source,omitempty"`
	From        *time.Time `json:"from,omitempty"`
	To          *time.Time `json:"to,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

type ActivityStats struct {
	TotalEvents    int64           `json:"totalEvents"`
	EventsToday    int64           `json:"eventsToday"`
	EventsThisHour int64           `json:"eventsThisHour"`
	UniqueActors   int64           `json:"uniqueActors"`
	ByLevel        map[Level]int64 `json:"byLevel"`
}

type Service struct {
	store ActivityStore
}

func New(store ActivityStore) *Service {
	return &Service{store: store}
}

func (s *Service) Log(ctx context.Context, event *ActivityEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if event.Source == "" {
		event.Source = "api"
	}
	if event.Properties == nil {
		event.Properties = json.RawMessage("{}")
	}
	if event.ActorType == "" {
		event.ActorType = "user"
	}
	return s.store.InsertActivity(ctx, event)
}

func (s *Service) Query(ctx context.Context, filter ActivityFilter) ([]ActivityEvent, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	return s.store.QueryActivities(ctx, filter)
}

func (s *Service) Count(ctx context.Context, filter ActivityFilter) (int, error) {
	return s.store.CountActivities(ctx, filter)
}

func (s *Service) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	before := time.Now().UTC().Add(-retention)
	return s.store.CleanupActivities(ctx, before)
}

func (s *Service) Stats(ctx context.Context) (*ActivityStats, error) {
	return s.store.GetActivityStats(ctx)
}

func (s *Service) NewEvent(event string) *ActivityEventBuilder {
	return &ActivityEventBuilder{
		event: &ActivityEvent{
			Event:     event,
			Timestamp: time.Now().UTC(),
			Level:     LevelInfo,
			Source:    "api",
			ActorType: "user",
		},
	}
}

type ActivityEventBuilder struct {
	event *ActivityEvent
}

func (b *ActivityEventBuilder) Actor(actorID, actorEmail, actorType string) *ActivityEventBuilder {
	b.event.ActorID = &actorID
	b.event.ActorEmail = &actorEmail
	b.event.ActorType = actorType
	return b
}

func (b *ActivityEventBuilder) Subject(subjectType, subjectID, subjectName string) *ActivityEventBuilder {
	b.event.SubjectType = &subjectType
	b.event.SubjectID = &subjectID
	b.event.SubjectName = &subjectName
	return b
}

func (b *ActivityEventBuilder) Level(level Level) *ActivityEventBuilder {
	b.event.Level = level
	return b
}

func (b *ActivityEventBuilder) Source(source string) *ActivityEventBuilder {
	b.event.Source = source
	return b
}

func (b *ActivityEventBuilder) Description(desc string) *ActivityEventBuilder {
	b.event.Description = desc
	return b
}

func (b *ActivityEventBuilder) IP(ip string) *ActivityEventBuilder {
	b.event.IP = &ip
	return b
}

func (b *ActivityEventBuilder) Properties(props map[string]any) *ActivityEventBuilder {
	data, _ := json.Marshal(props)
	b.event.Properties = data
	return b
}

func (b *ActivityEventBuilder) Save(ctx context.Context, s *Service) error {
	return s.Log(ctx, b.event)
}
