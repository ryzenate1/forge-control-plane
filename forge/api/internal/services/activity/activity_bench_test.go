package activity

import (
	"context"
	"testing"
	"time"
)

func BenchmarkActivityLog(b *testing.B) {
	store := &mockStore{}
	svc := New(store)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		svc.Log(ctx, &ActivityEvent{
			Event:  "server.created",
			Level:  LevelInfo,
			Source: "test",
		})
	}
}

func BenchmarkActivityQuery(b *testing.B) {
	store := &mockStore{}
	svc := New(store)
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		svc.Log(ctx, &ActivityEvent{
			Event:     "test.event",
			Level:     LevelInfo,
			Source:    "test",
			Timestamp: time.Now(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Query(ctx, ActivityFilter{Limit: 50})
	}
}
