package health

import (
	"context"
	"testing"
)

func BenchmarkHealthRunAll(b *testing.B) {
	svc := NewService("test")
	svc.AddCheck(&mockCheck{name: "db", status: StatusOK})
	svc.AddCheck(&mockCheck{name: "cache", status: StatusOK})
	svc.AddCheck(&mockCheck{name: "queue", status: StatusOK})
	svc.AddCheck(&mockCheck{name: "daemon", status: StatusWarning})
	svc.AddCheck(&mockCheck{name: "system", status: StatusOK})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		svc.RunAll(ctx)
	}
}

func BenchmarkHealthRunCheck(b *testing.B) {
	svc := NewService("test")
	svc.AddCheck(&mockCheck{name: "db", status: StatusOK})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.RunCheck(ctx, "db")
	}
}
