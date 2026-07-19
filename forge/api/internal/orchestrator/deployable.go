package orchestrator

import (
	"context"
	"errors"

	"gamepanel/forge/internal/store"
)

type deploymentStore interface {
	ServerProvisionTarget(context.Context, string) (store.ServerProvisionTarget, error)
}

func Ready(s interface{}) error {
	if s == nil {
		return errors.New("postgres is required")
	}
	return nil
}

func DaemonReady(s interface{}, r interface{}) error {
	if err := Ready(s); err != nil {
		return err
	}
	if r == nil {
		return errors.New("runtime is required")
	}
	return nil
}

func ServerProvisionTarget(ctx context.Context, store deploymentStore, serverID string) (store.ServerProvisionTarget, error) {
	return store.ServerProvisionTarget(ctx, serverID)
}
