package policies

import (
	"context"
	"gamepanel/forge/internal/store"
	"sync"
)

type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

type Policy interface {
	Name() string
	Can(ctx context.Context, user store.User, action Action, resource any) bool
}

type PolicyRegistry struct {
	policies map[string]Policy
	mu       sync.RWMutex
}

func NewPolicyRegistry() *PolicyRegistry {
	return &PolicyRegistry{
		policies: make(map[string]Policy),
	}
}

func (r *PolicyRegistry) Register(policy Policy) {
	if policy == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies[policy.Name()] = policy
}

func (r *PolicyRegistry) Can(ctx context.Context, user store.User, action Action, resource any, policyName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy, ok := r.policies[policyName]
	if !ok {
		return false
	}
	return policy.Can(ctx, user, action, resource)
}
