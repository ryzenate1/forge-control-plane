package replicamanager

import (
	"context"
	"log/slog"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// RemoteCommandDispatcher dispatches instance commands to beacon daemons via
// the daemon.Client HTTP client. It resolves node URLs and credentials from
// the store before each call.
type RemoteCommandDispatcher struct {
	store  *store.Store
	client *daemon.Client
	logger *slog.Logger
}

// NewRemoteCommandDispatcher creates a dispatcher backed by the daemon HTTP client.
func NewRemoteCommandDispatcher(store *store.Store, client *daemon.Client, logger *slog.Logger) *RemoteCommandDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteCommandDispatcher{
		store:  store,
		client: client,
		logger: logger,
	}
}

func (d *RemoteCommandDispatcher) StartInstance(ctx context.Context, req StartInstanceRequest) (CommandReceipt, error) {
	node, err := d.store.GetNode(ctx, req.NodeID)
	if err != nil {
		return CommandReceipt{}, err
	}

	token, err := d.store.GetNodeDaemonCredential(ctx, req.NodeID)
	if err != nil {
		return CommandReceipt{}, err
	}

	createReq := daemon.CreateRequest{
		ServerID:   req.InstanceID,
		MemoryMB:   int64(req.MemoryMB),
		CPUShares:  int64(req.CPU),
		DiskMB:     int64(req.DiskMB),
		Provider:   req.RuntimeProvider,
		NetworkName: "gamepanel",
	}
	if req.Image != "" {
		createReq.Image = req.Image
	}

	dispatchCtx := daemon.ContextWithCommandID(ctx, req.CommandID)
	_, err = d.client.CreateServer(dispatchCtx, node.BaseURL, token, createReq)
	if err != nil {
		d.logger.ErrorContext(ctx, "remote start instance failed",
			"instanceId", req.InstanceID,
			"nodeId", req.NodeID,
			"nodeUrl", node.BaseURL,
			"error", err,
		)
		return CommandReceipt{}, err
	}

	return CommandReceipt{
		CommandID: req.CommandID,
		Status:    "accepted",
	}, nil
}

func (d *RemoteCommandDispatcher) StopInstance(ctx context.Context, req StopInstanceRequest) (CommandReceipt, error) {
	node, err := d.store.GetNode(ctx, req.NodeID)
	if err != nil {
		return CommandReceipt{}, err
	}

	token, err := d.store.GetNodeDaemonCredential(ctx, req.NodeID)
	if err != nil {
		return CommandReceipt{}, err
	}

	dispatchCtx := daemon.ContextWithCommandID(ctx, req.CommandID)
	_, err = d.client.DeleteServer(dispatchCtx, node.BaseURL, token, req.InstanceID)
	if err != nil {
		d.logger.ErrorContext(ctx, "remote stop instance failed",
			"instanceId", req.InstanceID,
			"nodeId", req.NodeID,
			"nodeUrl", node.BaseURL,
			"error", err,
		)
		return CommandReceipt{}, err
	}

	return CommandReceipt{
		CommandID: req.CommandID,
		Status:    "accepted",
	}, nil
}

func (d *RemoteCommandDispatcher) GetCommandStatus(ctx context.Context, commandID string) (string, error) {
	logs, err := d.store.ListBeaconCommandLogs(ctx, store.BeaconCommandLogFilter{
		CommandID: commandID,
		Limit:     1,
	})
	if err != nil || len(logs) == 0 {
		return "unknown", err
	}
	return logs[0].Status, nil
}
