package policies

import (
	"context"
	"gamepanel/forge/internal/store"
)

type ServerPolicy struct {
	st *store.Store
}

func NewServerPolicy(st *store.Store) *ServerPolicy {
	return &ServerPolicy{st: st}
}

func (p *ServerPolicy) Name() string { return "server" }

func (p *ServerPolicy) Can(ctx context.Context, user store.User, action Action, resource any) bool {
	if user.Role == "admin" {
		return true
	}

	serverID, ok := resource.(string)
	if !ok {
		return false
	}

	switch action {
	case ActionRead:
		return p.checkPermission(ctx, user.ID, serverID, store.PermServerView)
	case ActionCreate:
		return true
	case ActionUpdate:
		return p.checkPermission(ctx, user.ID, serverID, store.PermServerSettings)
	case ActionDelete:
		return user.Role == "admin"
	}

	return false
}

func (p *ServerPolicy) checkPermission(ctx context.Context, userID, serverID, permission string) bool {
	allowed, err := p.st.UserCanAccessServer(ctx, serverID, userID, "", permission)
	return err == nil && allowed
}
