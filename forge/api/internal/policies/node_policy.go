package policies

import (
	"context"
	"gamepanel/forge/internal/store"
)

type NodePolicy struct{}

func NewNodePolicy() *NodePolicy {
	return &NodePolicy{}
}

func (p *NodePolicy) Name() string { return "node" }

func (p *NodePolicy) Can(ctx context.Context, user store.User, action Action, resource any) bool {
	return user.Role == "admin"
}
