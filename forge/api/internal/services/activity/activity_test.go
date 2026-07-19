package activity

import (
	"context"
	"testing"
	"time"
)

type mockStore struct {
	events []ActivityEvent
}

func (m *mockStore) InsertActivity(ctx context.Context, event *ActivityEvent) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockStore) QueryActivities(ctx context.Context, filter ActivityFilter) ([]ActivityEvent, error) {
	return m.events, nil
}

func (m *mockStore) CountActivities(ctx context.Context, filter ActivityFilter) (int, error) {
	return len(m.events), nil
}

func (m *mockStore) CleanupActivities(ctx context.Context, before time.Time) (int64, error) {
	count := int64(0)
	var remaining []ActivityEvent
	for _, e := range m.events {
		if e.Timestamp.Before(before) {
			count++
		} else {
			remaining = append(remaining, e)
		}
	}
	m.events = remaining
	return count, nil
}

func (m *mockStore) GetActivityStats(ctx context.Context) (*ActivityStats, error) {
	return &ActivityStats{
		TotalEvents: int64(len(m.events)),
		ByLevel:     map[Level]int64{LevelInfo: int64(len(m.events))},
	}, nil
}

func TestActivityService_Log(t *testing.T) {
	store := &mockStore{}
	svc := New(store)

	err := svc.Log(context.Background(), &ActivityEvent{
		Event:  "test.event",
		Level:  LevelInfo,
		Source: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}

	if store.events[0].Event != "test.event" {
		t.Errorf("expected event 'test.event', got %q", store.events[0].Event)
	}

	if store.events[0].Level != LevelInfo {
		t.Errorf("expected level 'info', got %q", store.events[0].Level)
	}
}

func TestActivityService_Query(t *testing.T) {
	store := &mockStore{}
	svc := New(store)

	svc.Log(context.Background(), &ActivityEvent{Event: "e1", Timestamp: time.Now()})
	svc.Log(context.Background(), &ActivityEvent{Event: "e2", Timestamp: time.Now()})

	events, err := svc.Query(context.Background(), ActivityFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestActivityEventBuilder(t *testing.T) {
	store := &mockStore{}
	svc := New(store)

	actorID := "user-1"
	actorEmail := "test@example.com"

	svc.NewEvent("server.created").
		Actor(actorID, actorEmail, "user").
		Subject("server", "srv-1", "My Server").
		Level(LevelInfo).
		Source("api").
		Description("Server was created").
		IP("192.168.1.1").
		Properties(map[string]any{"template": "minecraft"}).
		Save(context.Background(), svc)

	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}

	e := store.events[0]
	if e.Event != "server.created" {
		t.Errorf("expected 'server.created', got %q", e.Event)
	}
	if e.ActorID == nil || *e.ActorID != actorID {
		t.Errorf("expected actor %q, got %v", actorID, e.ActorID)
	}
	if e.Level != LevelInfo {
		t.Errorf("expected LevelInfo, got %q", e.Level)
	}
}

func TestActivityCleanup(t *testing.T) {
	store := &mockStore{}
	svc := New(store)

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now()

	svc.Log(context.Background(), &ActivityEvent{Event: "old", Timestamp: old})
	svc.Log(context.Background(), &ActivityEvent{Event: "recent", Timestamp: recent})

	count, err := svc.Cleanup(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("expected 1 cleanup, got %d", count)
	}

	remaining, _ := svc.Query(context.Background(), ActivityFilter{})
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(remaining))
	}
}
