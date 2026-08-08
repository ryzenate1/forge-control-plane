package orchestrator

import (
	"context"
	"errors"

	"gamepanel/forge/internal/events"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"
)

type SuspensionManager interface {
	SuspendServer(ctx context.Context, serverID string) error
	UnsuspendServer(ctx context.Context, serverID string) error
}

type SuspensionStore interface {
	GetServer(ctx context.Context, serverID string) (store.Server, error)
	SetServerSuspension(ctx context.Context, serverID string, suspended bool) error
	CompareAndSetServerSuspension(ctx context.Context, serverID string, expected, suspended bool) (bool, error)
	ServerControlTarget(ctx context.Context, serverID string) (store.ServerControlTarget, error)
}

type SuspensionRuntime interface {
	StopServer(ctx context.Context, target gpruntime.Target) (gpruntime.PowerResponse, error)
}

func SuspendServer(ctx context.Context, store SuspensionStore, runtime SuspensionRuntime, publisher events.Publisher, serverID string) error {
	if store == nil {
		return errors.New("store is required")
	}
	if runtime == nil {
		return errors.New("runtime is required")
	}

	server, err := store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	if server.Suspended {
		return errors.New("server already suspended")
	}
	target, err := store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return err
	}
	changed, err := store.CompareAndSetServerSuspension(ctx, serverID, false, true)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("server suspension state changed concurrently")
	}
	rollback := true
	defer func() {
		if rollback {
			_, _ = store.CompareAndSetServerSuspension(context.WithoutCancel(ctx), serverID, true, false)
		}
	}()

	if _, err := runtime.StopServer(ctx, gpruntime.Target{
		NodeURL:   target.NodeURL,
		NodeToken: target.NodeToken,
		ServerID:  target.ServerID,
	}); err != nil {
		return err
	}

	rollback = false

	if publisher != nil {
		if err := publisher.Publish(ctx, events.NewEnvelope(events.EventServerStopped, "orchestrator", "server", serverID, map[string]any{
			"suspended": true,
		})); err != nil {
			return err
		}
	}

	return nil
}

func UnsuspendServer(ctx context.Context, store SuspensionStore, publisher events.Publisher, serverID string) error {
	if store == nil {
		return errors.New("store is required")
	}

	server, err := store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	if !server.Suspended {
		return errors.New("server not suspended")
	}

	changed, err := store.CompareAndSetServerSuspension(ctx, serverID, true, false)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("server suspension state changed concurrently")
	}

	if publisher != nil {
		if err := publisher.Publish(ctx, events.NewEnvelope(events.EventDesiredStateChanged, "orchestrator", "server", serverID, map[string]any{
			"suspended": false,
		})); err != nil {
			return err
		}
	}

	return nil
}
