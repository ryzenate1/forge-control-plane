package replicamanager

import (
	"context"
	"fmt"
	"log/slog"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// BeaconHTTPClient implements BeaconClient by dispatching commands to Beacon
// nodes via HTTP using the daemon client infrastructure.
type BeaconHTTPClient struct {
	store        *store.Store
	daemonClient *daemon.Client
	logger       *slog.Logger
}

// NewBeaconHTTPClient creates a new BeaconHTTPClient that can dispatch commands
// to Beacon nodes via HTTP.
func NewBeaconHTTPClient(store *store.Store, daemonClient *daemon.Client, _ string, logger *slog.Logger) *BeaconHTTPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &BeaconHTTPClient{
		store:        store,
		daemonClient: daemonClient,
		logger:       logger,
	}
}

// DispatchCommand sends a provisioning command to a Beacon node via HTTP.
// It resolves the node's URL and credentials from the store, then dispatches
// the appropriate command based on the command type.
func (c *BeaconHTTPClient) DispatchCommand(ctx context.Context, nodeID string, commandID string, commandType InstanceCommandType, payload map[string]any) error {
	if c.daemonClient == nil {
		return fmt.Errorf("daemon client is nil")
	}

	node, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeID, err)
	}

	token, err := c.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to get node credentials for %s: %w", nodeID, err)
	}

	// Map replicamanager command types to daemon power signals
	switch commandType {
	case StartInstanceCommand:
		// For start commands, we need to extract the server configuration from payload
		if payload == nil {
			payload = make(map[string]any)
		}

		// Build CreateRequest from payload
		instanceID, _ := payload["instanceId"].(string)
		memoryMb, _ := payload["memoryMb"].(int)
		cpu, _ := payload["cpu"].(int)
		diskMb, _ := payload["diskMb"].(int)
		runtimeProvider, _ := payload["runtimeProvider"].(string)
		createReq := daemon.CreateRequest{
			ServerID:    instanceID,
			MemoryMB:    int64(memoryMb),
			CPUShares:   int64(cpu),
			DiskMB:      int64(diskMb),
			Provider:    runtimeProvider,
			NetworkName: "gamepanel",
		}

		if image, ok := payload["image"].(string); ok && image != "" {
			createReq.Image = image
		}

		// Use the commandID as idempotency key
		dispatchCtx := daemon.ContextWithCommandID(ctx, commandID)
		_, err = c.daemonClient.CreateServer(dispatchCtx, node.BaseURL, token, createReq)
		if err != nil {
			return fmt.Errorf("failed to dispatch start command to node %s: %w", nodeID, err)
		}

	case StopInstanceCommand:
		instanceID, _ := payload["instanceId"].(string)
		dispatchCtx := daemon.ContextWithCommandID(ctx, commandID)
		_, err = c.daemonClient.DeleteServer(dispatchCtx, node.BaseURL, token, instanceID)
		if err != nil {
			return fmt.Errorf("failed to dispatch stop command to node %s: %w", nodeID, err)
		}

	default:
		return fmt.Errorf("unsupported command type: %s", commandType)
	}

	c.logger.DebugContext(ctx, "beacon command dispatched",
		"nodeId", nodeID,
		"commandId", commandID,
		"commandType", commandType,
		"nodeUrl", node.BaseURL,
	)

	return nil
}
