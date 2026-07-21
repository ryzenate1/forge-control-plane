package replicamanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// BeaconHTTPClient implements BeaconClient by dispatching commands to Beacon
// nodes via HTTP using the daemon client infrastructure.
type BeaconHTTPClient struct {
	store        *store.Store
	daemonClient *daemon.Client
	baseURL      string
	logger       *slog.Logger
}

// NewBeaconHTTPClient creates a new BeaconHTTPClient that can dispatch commands
// to Beacon nodes via HTTP.
func NewBeaconHTTPClient(store *store.Store, daemonClient *daemon.Client, baseURL string, logger *slog.Logger) *BeaconHTTPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &BeaconHTTPClient{
		store:        store,
		daemonClient: daemonClient,
		baseURL:      strings.TrimRight(baseURL, "/"),
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

// dispatchViaBeaconAPI dispatches commands directly to the Beacon API endpoint
// This is an alternative approach using the Beacon operations queue endpoint
func (c *BeaconHTTPClient) dispatchViaBeaconAPI(ctx context.Context, nodeID string, commandID string, commandType InstanceCommandType, payload map[string]any) error {
	if c.baseURL == "" {
		return fmt.Errorf("beacon base URL is not configured")
	}

	token, err := c.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to get node credentials for %s: %w", nodeID, err)
	}

	// Map command type to Beacon operation type
	var opType string
	switch commandType {
	case StartInstanceCommand:
		opType = "start"
	case StopInstanceCommand:
		opType = "stop"
	default:
		return fmt.Errorf("unsupported command type: %s", commandType)
	}

	// Build request payload
	requestPayload := map[string]any{
		"signal": opType,
	}

	// Add server ID if available in payload
	if instanceID, ok := payload["instanceId"].(string); ok {
		requestPayload["serverId"] = instanceID
	}

	// Marshal payload
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL for the specific server's power endpoint
	// Beacon API: POST /servers/{serverId}/power
	serverID, _ := payload["instanceId"].(string)
	url := fmt.Sprintf("%s/servers/%s/power", c.baseURL, serverID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Forge-Command-ID", commandID)

	// Execute request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		var errorBody map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&errorBody); err == nil {
			if errorMsg, ok := errorBody["error"]; ok {
				return fmt.Errorf("beacon API error: %s", errorMsg)
			}
		}
		return fmt.Errorf("beacon API returned status %d", resp.StatusCode)
	}

	return nil
}
