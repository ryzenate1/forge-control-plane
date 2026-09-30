package replicamanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// InstanceCommandType identifies the kind of command dispatched to beacon.
type InstanceCommandType string

const (
	StartInstanceCommand InstanceCommandType = "start_replica"
	StopInstanceCommand  InstanceCommandType = "stop_replica"
)

// AppDeploymentStatus tracks the overall deployment health of a replica application.
type AppDeploymentStatus string

const (
	AppDeploymentStatusPending   AppDeploymentStatus = "pending"
	AppDeploymentStatusDeploying AppDeploymentStatus = "deploying"
	AppDeploymentStatusRunning   AppDeploymentStatus = "running"
	AppDeploymentStatusDegraded  AppDeploymentStatus = "degraded"
	AppDeploymentStatusFailed    AppDeploymentStatus = "failed"
)

// CommandReceipt is returned when a command is enqueued/dispatched to beacon.
type CommandReceipt struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}

// StartInstanceRequest carries all data needed to start a single replica on a node.
type StartInstanceRequest struct {
	AppID           string `json:"appId"`
	InstanceID      string `json:"instanceId"`
	NodeID          string `json:"nodeId"`
	Index           int    `json:"index"`
	CPU             int    `json:"cpu"`
	MemoryMB        int    `json:"memoryMb"`
	DiskMB          int    `json:"diskMb"`
	RuntimeProvider string `json:"runtimeProvider"`
	CommandID       string `json:"commandId"`
	OperationID     string `json:"operationId"`
	Generation      int    `json:"generation"`
	Image           string `json:"image,omitempty"`
}

// StopInstanceRequest carries data to stop a single replica on a node.
type StopInstanceRequest struct {
	InstanceID  string `json:"instanceId"`
	NodeID      string `json:"nodeId"`
	CommandID   string `json:"commandId"`
	OperationID string `json:"operationId"`
}

// InstanceCommandDispatcher abstracts the mechanism for sending commands to beacon daemons.
// Implementations may use HTTP (daemon.Client), a queue, or a test double.
type InstanceCommandDispatcher interface {
	// StartInstance dispatches a command to start a replica on the given node.
	// Must be idempotent: the same CommandID can be sent multiple times safely.
	StartInstance(ctx context.Context, req StartInstanceRequest) (CommandReceipt, error)

	// StopInstance dispatches a command to stop a replica on the given node.
	// Must be idempotent.
	StopInstance(ctx context.Context, req StopInstanceRequest) (CommandReceipt, error)

	// GetCommandStatus returns the current status of a previously dispatched command.
	GetCommandStatus(ctx context.Context, commandID string) (string, error)
}

// ErrDispatcherNotConfigured is returned when a replica command has nowhere to
// go because no beacon dispatcher was wired.
var ErrDispatcherNotConfigured = errors.New("beacon command dispatcher is not configured")

// UnavailableCommandDispatcher is the fallback used when no real dispatcher is
// wired. It refuses every command.
//
// It replaces an earlier no-op that answered "pending" with a nil error, which
// made an unwired control plane indistinguishable from a working one: replicas
// were recorded as commanded, no beacon ever heard about them, and the status
// stayed "pending" forever with nothing in flight to change it.
type UnavailableCommandDispatcher struct{}

func (UnavailableCommandDispatcher) StartInstance(_ context.Context, req StartInstanceRequest) (CommandReceipt, error) {
	return CommandReceipt{}, fmt.Errorf("start replica %s on node %s: %w", req.InstanceID, req.NodeID, ErrDispatcherNotConfigured)
}

func (UnavailableCommandDispatcher) StopInstance(_ context.Context, req StopInstanceRequest) (CommandReceipt, error) {
	return CommandReceipt{}, fmt.Errorf("stop replica %s on node %s: %w", req.InstanceID, req.NodeID, ErrDispatcherNotConfigured)
}

func (UnavailableCommandDispatcher) GetCommandStatus(_ context.Context, _ string) (string, error) {
	return "", ErrDispatcherNotConfigured
}

// instanceCommandID builds an idempotent command identifier scoped to an instance,
// operation type, and generation.
func instanceCommandID(instanceID string, typ InstanceCommandType, generation int, operationID string) string {
	return fmt.Sprintf("%s-%s-%d-%s", instanceID, string(typ), generation, operationID)
}

// newOperationID creates a new unique operation identifier for a deploy/scale operation.
func newOperationID() string {
	return uuid.NewString()
}
