package policies

import (
	"context"
	"gamepanel/forge/internal/store"
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
}

func NewPolicyRegistry() *PolicyRegistry {
	return &PolicyRegistry{
		policies: make(map[string]Policy),
	}
}

func (r *PolicyRegistry) Register(policy Policy) {
	r.policies[policy.Name()] = policy
}

func (r *PolicyRegistry) Can(ctx context.Context, user store.User, action Action, resource any, policyName string) bool {
	policy, ok := r.policies[policyName]
	if !ok {
		return false
	}
	return policy.Can(ctx, user, action, resource)
}
