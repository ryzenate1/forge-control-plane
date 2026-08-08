package replicamanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// BeaconHTTPClient implements BeaconClient by dispatching commands to Beacon
// nodes via HTTP using the daemon client infrastructure.
type BeaconHTTPClient struct {
	store        *store.Store
	daemonClient *daemon.Client
	logger       *slog.Logger
	mu           sync.Mutex
	nodes        map[string]nodeCircuitState
}

type nodeCircuitState struct {
	failures    int
	openUntil   time.Time
	lastHealthy time.Time
	version     string
}

// NewBeaconHTTPClient creates a new BeaconHTTPClient that can dispatch commands
// to Beacon nodes via HTTP.
func NewBeaconHTTPClient(store *store.Store, daemonClient *daemon.Client, logger *slog.Logger) *BeaconHTTPClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &BeaconHTTPClient{
		store:        store,
		daemonClient: daemonClient,
		logger:       logger,
		nodes:        make(map[string]nodeCircuitState),
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
	staleAfter := time.Minute
	if node.HeartbeatInterval > 0 && time.Duration(node.HeartbeatInterval*3)*time.Second > staleAfter {
		staleAfter = time.Duration(node.HeartbeatInterval*3) * time.Second
	}
	if strings.EqualFold(node.HeartbeatState, "offline") ||
		!node.LastHeartbeatAt.IsZero() && time.Since(node.LastHeartbeatAt) > staleAfter {
		c.recordFailure(nodeID)
		return fmt.Errorf("Beacon node %s heartbeat is stale or offline", nodeID)
	}

	token, err := c.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("failed to get node credentials for %s: %w", nodeID, err)
	}
	if err := c.preflight(ctx, nodeID, node.BaseURL, token); err != nil {
		c.recordFailure(nodeID)
		return fmt.Errorf("Beacon node %s is not ready: %w", nodeID, err)
	}

	// Map replicamanager command types to daemon power signals
	switch commandType {
	case StartInstanceCommand:
		// For start commands, we need to extract the server configuration from payload
		if payload == nil {
			payload = make(map[string]any)
		}

		// Build CreateRequest from payload
		// JSON numbers are decoded as float64 by json.Unmarshal, so we
		// must accept both int and float64 to avoid silent zero values.
		instanceID, _ := payload["instanceId"].(string)
		memoryMb := extractInt(payload, "memoryMb")
		cpu := extractInt(payload, "cpu")
		diskMb := extractInt(payload, "diskMb")
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
			c.recordFailure(nodeID)
			return fmt.Errorf("failed to dispatch start command to node %s: %w", nodeID, err)
		}

	case StopInstanceCommand:
		instanceID, _ := payload["instanceId"].(string)
		dispatchCtx := daemon.ContextWithCommandID(ctx, commandID)
		_, err = c.daemonClient.DeleteServer(dispatchCtx, node.BaseURL, token, instanceID)
		if err != nil {
			c.recordFailure(nodeID)
			return fmt.Errorf("failed to dispatch stop command to node %s: %w", nodeID, err)
		}

	default:
		return fmt.Errorf("unsupported command type: %s", commandType)
	}
	c.recordSuccess(nodeID)

	c.logger.DebugContext(ctx, "beacon command dispatched",
		"nodeId", nodeID,
		"commandId", commandID,
		"commandType", commandType,
	)

	return nil
}

func (c *BeaconHTTPClient) preflight(ctx context.Context, nodeID, baseURL, token string) error {
	now := time.Now()
	c.mu.Lock()
	state := c.nodes[nodeID]
	if now.Before(state.openUntil) {
		c.mu.Unlock()
		return fmt.Errorf("circuit is open until %s", state.openUntil.UTC().Format(time.RFC3339))
	}
	if now.Sub(state.lastHealthy) < 15*time.Second && state.version != "" {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	capabilities, err := c.daemonClient.GetNodeCapabilities(checkCtx, baseURL, token)
	if err != nil {
		return fmt.Errorf("health/capability check failed: %w", err)
	}
	if err := requireCompatibleBeacon(capabilities.BeaconVersion); err != nil {
		return err
	}
	c.mu.Lock()
	state = c.nodes[nodeID]
	state.failures = 0
	state.openUntil = time.Time{}
	state.lastHealthy = now
	state.version = capabilities.BeaconVersion
	c.nodes[nodeID] = state
	c.mu.Unlock()
	return nil
}

func (c *BeaconHTTPClient) recordFailure(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.nodes[nodeID]
	state.failures++
	state.lastHealthy = time.Time{}
	if state.failures >= 3 {
		state.openUntil = time.Now().Add(30 * time.Second)
	}
	c.nodes[nodeID] = state
}

func (c *BeaconHTTPClient) recordSuccess(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.nodes[nodeID]
	state.failures = 0
	state.openUntil = time.Time{}
	state.lastHealthy = time.Now()
	c.nodes[nodeID] = state
}

func (c *BeaconHTTPClient) VerifyInstance(ctx context.Context, nodeID, instanceID string) error {
	if c.daemonClient == nil {
		return errors.New("daemon client is nil")
	}
	node, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	token, err := c.store.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return err
	}
	if err := c.preflight(ctx, nodeID, node.BaseURL, token); err != nil {
		c.recordFailure(nodeID)
		return err
	}
	if _, err := c.daemonClient.Stats(ctx, node.BaseURL, token, instanceID); err != nil {
		c.recordFailure(nodeID)
		return fmt.Errorf("query Beacon instance stats: %w", err)
	}
	c.recordSuccess(nodeID)
	return nil
}

func requireCompatibleBeacon(version string) error {
	value := strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.SplitN(value, ".", 3)
	if len(parts) != 3 {
		return fmt.Errorf("Beacon returned invalid version %q", version)
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	patchText := strings.SplitN(parts[2], "-", 2)[0]
	patch, patchErr := strconv.Atoi(patchText)
	if majorErr != nil || minorErr != nil || patchErr != nil || major < 0 || minor < 0 || patch < 0 {
		return fmt.Errorf("Beacon returned invalid version %q", version)
	}
	if major != 0 || minor < 1 {
		return fmt.Errorf("Beacon version %q is incompatible; supported range is >=0.1.0 <1.0.0", version)
	}
	return nil
}

// extractInt extracts an int value from a map that was decoded from JSON.
// json.Unmarshal decodes all JSON numbers as float64, so a bare .(int) assertion
// would fail and silently return 0. This helper accepts int, float64, and
// json.Number to safely handle values from any decoding path.
func extractInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	default:
		return 0
	}
}
