package policies

import (
	"context"
	"gamepanel/forge/internal/store"
	"testing"
)

func TestUserPolicy(t *testing.T) {
	policy := NewUserPolicy()
	ctx := context.Background()

	admin := store.User{Role: "admin", ID: "admin-1"}
	user := store.User{Role: "user", ID: "user-1"}

	tests := []struct {
		name     string
		user     store.User
		action   Action
		resource any
		allowed  bool
	}{
		{"admin can create users", admin, ActionCreate, nil, true},
		{"user cannot create users", user, ActionCreate, nil, false},
		{"admin can delete users", admin, ActionDelete, "user-2", true},
		{"user cannot delete users", user, ActionDelete, "user-2", false},
		{"admin can read any user", admin, ActionRead, "user-2", true},
		{"user can read any user", user, ActionRead, "user-2", true},
		{"admin can update any user", admin, ActionUpdate, "user-2", true},
		{"user can update self", user, ActionUpdate, "user-1", true},
		{"user cannot update others", user, ActionUpdate, "user-2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.Can(ctx, tt.user, tt.action, tt.resource)
			if got != tt.allowed {
				t.Errorf("Can(%v, %v, %v) = %v, want %v", tt.user.Role, tt.action, tt.resource, got, tt.allowed)
			}
		})
	}
}

func TestNodePolicy(t *testing.T) {
	policy := NewNodePolicy()
	ctx := context.Background()

	if !policy.Can(ctx, store.User{Role: "admin"}, ActionCreate, nil) {
		t.Error("admin should be able to create nodes")
	}

	if policy.Can(ctx, store.User{Role: "user"}, ActionCreate, nil) {
		t.Error("user should NOT be able to create nodes")
	}
}

func TestPolicyRegistry(t *testing.T) {
	registry := NewPolicyRegistry()
	registry.Register(NewUserPolicy())
	registry.Register(NewNodePolicy())

	ctx := context.Background()
	admin := store.User{Role: "admin"}
	user := store.User{Role: "user"}

	if !registry.Can(ctx, admin, ActionCreate, nil, "user") {
		t.Error("admin should be able to create users via registry")
	}

	if registry.Can(ctx, user, ActionCreate, nil, "node") {
		t.Error("user should NOT be able to create nodes via registry")
	}
}
