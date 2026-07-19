package orchestrator

import (
	"context"

	"gamepanel/forge/internal/domain"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"
)

type ServerLifecycle interface {
	CreateServer(ctx context.Context, req store.CreateServerRequest, placementReq domain.PlacementRequest) (store.Server, domain.PlacementDecision, error)
	InstallServer(ctx context.Context, serverID string) (gpruntime.InstallResponse, error)
	ReinstallServer(ctx context.Context, serverID string) (gpruntime.InstallResponse, error)
	SyncServerConfiguration(ctx context.Context, serverID string) error
	DeleteServer(ctx context.Context, serverID string, force bool) (gpruntime.PowerResponse, error)
}

type PowerOperations interface {
	StartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	StopServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	RestartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	KillServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	RequestServerPower(ctx context.Context, serverID, signal string) (gpruntime.PowerResponse, domain.ServerDesiredState, error)
}

type CapacityViewer interface {
	NodeCapacity(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error)
	RefreshServerActualState(ctx context.Context, serverID string) (domain.ServerActualState, error)
}

type NodeReconciler interface {
	ReconcileNode(ctx context.Context, nodeID string) error
}
