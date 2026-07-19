package policies

import (
	"context"
	"gamepanel/forge/internal/store"
)

type UserPolicy struct{}

func NewUserPolicy() *UserPolicy {
	return &UserPolicy{}
}

func (p *UserPolicy) Name() string { return "user" }

func (p *UserPolicy) Can(ctx context.Context, user store.User, action Action, resource any) bool {
	switch action {
	case ActionCreate:
		return user.Role == "admin"
	case ActionRead:
		return true
	case ActionUpdate:
		targetID, ok := resource.(string)
		if !ok {
			return false
		}
		return user.Role == "admin" || user.ID == targetID
	case ActionDelete:
		return user.Role == "admin"
	}
	return false
}
