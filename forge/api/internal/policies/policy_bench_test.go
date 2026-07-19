package policies

import (
	"context"
	"gamepanel/forge/internal/store"
	"testing"
)

func BenchmarkUserPolicyCheck(b *testing.B) {
	policy := NewUserPolicy()
	ctx := context.Background()
	admin := store.User{Role: "admin", ID: "admin-1"}
	user := store.User{Role: "user", ID: "user-1"}

	b.Run("admin-create", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			policy.Can(ctx, admin, ActionCreate, nil)
		}
	})

	b.Run("user-read-self", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			policy.Can(ctx, user, ActionUpdate, "user-1")
		}
	})

	b.Run("user-read-other", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			policy.Can(ctx, user, ActionUpdate, "user-2")
		}
	})
}

func BenchmarkRegistryLookup(b *testing.B) {
	registry := NewPolicyRegistry()
	registry.Register(NewUserPolicy())
	registry.Register(NewNodePolicy())
	ctx := context.Background()
	admin := store.User{Role: "admin"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Can(ctx, admin, ActionCreate, nil, "user")
	}
}
