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
	// preflightCacheHits counts dispatches that skipped the capability
	// round-trip because a recent check was still within preflightHealthyTTL.
	preflightCacheHits uint64
}

// PreflightCacheHits returns the number of preflight checks served from the
// healthy cache, keyed by node ID. Exposed so health endpoints and tests can
// observe how often the capability round-trip is skipped.
func (c *BeaconHTTPClient) PreflightCacheHits() map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]uint64, len(c.nodes))
	for id, state := range c.nodes {
		if state.preflightCacheHits > 0 {
			out[id] = state.preflightCacheHits
		}
	}
	return out
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
	if strings.EqualFold(node.HeartbeatState, "offline") {
		c.recordFailure(nodeID)
		return fmt.Errorf("Beacon node %s is offline", nodeID)
	}
	// A node that has never reported a heartbeat has unknown health, and
	// unknown is not healthy: it must not receive workloads until it checks
	// in. (Previously a zero LastHeartbeatAt skipped the staleness check
	// entirely and the node was treated as fresh.)
	if node.LastHeartbeatAt.IsZero() {
		c.recordFailure(nodeID)
		return fmt.Errorf("Beacon node %s has never reported a heartbeat", nodeID)
	}
	if time.Since(node.LastHeartbeatAt) > staleAfter {
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
		// A missing or mistyped field is a caller bug, not a zero value:
		// extractInt reports ok=false so dispatch fails loudly instead of
		// provisioning a container with 0 CPU/memory/disk.
		instanceID, _ := payload["instanceId"].(string)
		memoryMb, ok := extractInt(payload, "memoryMb")
		if !ok {
			return fmt.Errorf("start command payload is missing required field %q", "memoryMb")
		}
		cpu, ok := extractInt(payload, "cpu")
		if !ok {
			return fmt.Errorf("start command payload is missing required field %q", "cpu")
		}
		diskMb, ok := extractInt(payload, "diskMb")
		if !ok {
			return fmt.Errorf("start command payload is missing required field %q", "diskMb")
		}
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

// preflightHealthyTTL bounds how long a successful capability check is trusted
// without re-checking the node. A shorter window re-detects a dead node
// sooner; a longer window spares the node a capability round-trip per
// dispatch. 10s keeps dispatch latency low while ensuring a node that went
// dark is re-probed on the next command batch.
const preflightHealthyTTL = 10 * time.Second

func (c *BeaconHTTPClient) preflight(ctx context.Context, nodeID, baseURL, token string) error {
	now := time.Now()
	c.mu.Lock()
	state := c.nodes[nodeID]
	if now.Before(state.openUntil) {
		c.mu.Unlock()
		return fmt.Errorf("circuit is open until %s", state.openUntil.UTC().Format(time.RFC3339))
	}
	if now.Sub(state.lastHealthy) < preflightHealthyTTL && state.version != "" {
		state.preflightCacheHits++
		c.nodes[nodeID] = state
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
	// From here the node has answered. What it says about the instance is an
	// instance fact and must not be charged to the node's circuit breaker: an
	// instance that exited on its own used to count as a node failure, and three
	// crash loops on one healthy machine took that machine out of rotation.
	state, err := c.daemonClient.ContainerState(ctx, node.BaseURL, token, instanceID)
	if errors.Is(err, daemon.ErrContainerStateUnsupported) {
		// An older Beacon exposes no lifecycle state. Telemetry presence is the
		// best it can offer, reported as such rather than as a verified running
		// instance.
		if _, statsErr := c.daemonClient.Stats(ctx, node.BaseURL, token, instanceID); statsErr != nil {
			c.recordSuccess(nodeID)
			return fmt.Errorf("beacon exposes no container lifecycle state and instance stats failed: %w", statsErr)
		}
		c.recordSuccess(nodeID)
		return nil
	}
	if err != nil {
		c.recordFailure(nodeID)
		return fmt.Errorf("query Beacon instance state: %w", err)
	}
	c.recordSuccess(nodeID)
	if !state.Exists {
		return fmt.Errorf("instance %s is not present on node %s", instanceID, nodeID)
	}
	if !state.Running {
		status := state.Status
		if status == "" {
			status = "not running"
		}
		return fmt.Errorf("instance %s is %s on node %s", instanceID, status, nodeID)
	}
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
// would fail and silently return 0. This helper accepts int, int64, float64,
// and json.Number to safely handle values from any decoding path.
//
// It returns ok=false when the key is missing or the value is not a number,
// so callers can reject the payload instead of provisioning with a silent zero.
func extractInt(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}
