package auditlog

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryAuditLogger_Log(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	err := logger.Log(ctx, AuditEvent{
		ID:           "1",
		UserID:       "user1",
		Action:       "server.create",
		ResourceType: "server",
		ResourceID:   "srv1",
		Details:      map[string]any{"name": "test"},
		IP:           "127.0.0.1",
		UserAgent:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := logger.Query(ctx, AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "1" {
		t.Fatalf("expected ID 1, got %s", events[0].ID)
	}
}

func TestInMemoryAuditLogger_Query_FilterByUserID(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	_ = logger.Log(ctx, AuditEvent{ID: "1", UserID: "user1", Action: "a"})
	_ = logger.Log(ctx, AuditEvent{ID: "2", UserID: "user2", Action: "b"})
	_ = logger.Log(ctx, AuditEvent{ID: "3", UserID: "user1", Action: "c"})

	events, err := logger.Query(ctx, AuditFilter{UserID: "user1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for user1, got %d", len(events))
	}
}

func TestInMemoryAuditLogger_Query_FilterByAction(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	_ = logger.Log(ctx, AuditEvent{ID: "1", Action: "server.create"})
	_ = logger.Log(ctx, AuditEvent{ID: "2", Action: "server.delete"})
	_ = logger.Log(ctx, AuditEvent{ID: "3", Action: "server.create"})

	events, err := logger.Query(ctx, AuditFilter{Action: "server.create"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 create events, got %d", len(events))
	}
}

func TestInMemoryAuditLogger_Query_FilterByResourceType(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	_ = logger.Log(ctx, AuditEvent{ID: "1", ResourceType: "server"})
	_ = logger.Log(ctx, AuditEvent{ID: "2", ResourceType: "user"})

	events, err := logger.Query(ctx, AuditFilter{ResourceType: "server"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 server event, got %d", len(events))
	}
}

func TestInMemoryAuditLogger_Query_FilterByTimeRange(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	now := time.Now()
	_ = logger.Log(ctx, AuditEvent{ID: "1", Timestamp: now.Add(-3 * time.Hour)})
	_ = logger.Log(ctx, AuditEvent{ID: "2", Timestamp: now.Add(-1 * time.Hour)})
	_ = logger.Log(ctx, AuditEvent{ID: "3", Timestamp: now})

	events, err := logger.Query(ctx, AuditFilter{
		Since: now.Add(-2 * time.Hour),
		Until: now.Add(-30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event in range, got %d", len(events))
	}
	if events[0].ID != "2" {
		t.Fatalf("expected event ID 2, got %s", events[0].ID)
	}
}

func TestInMemoryAuditLogger_Query_Limit(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = logger.Log(ctx, AuditEvent{ID: string(rune('a' + i)), Action: "test"})
	}

	events, err := logger.Query(ctx, AuditFilter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestInMemoryAuditLogger_RingBuffer(t *testing.T) {
	logger := NewInMemoryAuditLogger(5)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = logger.Log(ctx, AuditEvent{
			ID:     string(rune('a' + i)),
			Action: "test",
		})
	}

	events, err := logger.Query(ctx, AuditFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events (ring buffer), got %d", len(events))
	}
}

func TestInMemoryAuditLogger_DefaultCapacity(t *testing.T) {
	logger := NewInMemoryAuditLogger(0)
	if logger.capacity != 10000 {
		t.Fatalf("expected default capacity 10000, got %d", logger.capacity)
	}
}

func TestInMemoryAuditLogger_AutoTimestamp(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	before := time.Now()
	_ = logger.Log(ctx, AuditEvent{ID: "1"})
	after := time.Now()

	events, _ := logger.Query(ctx, AuditFilter{})
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	if events[0].Timestamp.Before(before) || events[0].Timestamp.After(after) {
		t.Fatal("expected auto-assigned timestamp within range")
	}
}

func TestInMemoryAuditLogger_EmptyQuery(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	events, err := logger.Query(ctx, AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if events != nil {
		t.Fatalf("expected nil events for empty logger, got %v", events)
	}
}
