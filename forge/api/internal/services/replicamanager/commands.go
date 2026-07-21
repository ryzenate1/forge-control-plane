package replicamanager

import (
	"context"
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

// NoopCommandDispatcher is a stub implementation of InstanceCommandDispatcher that
// returns a CommandReceipt with status "pending" without dispatching to any beacon.
// It is a safe default for when beacon integration is not fully wired yet.
type NoopCommandDispatcher struct{}

// StartInstance returns a pending receipt without performing any actual dispatch.
func (n NoopCommandDispatcher) StartInstance(_ context.Context, req StartInstanceRequest) (CommandReceipt, error) {
	return CommandReceipt{
		CommandID: req.CommandID,
		Status:    "pending",
	}, nil
}

// StopInstance returns a pending receipt without performing any actual dispatch.
func (n NoopCommandDispatcher) StopInstance(_ context.Context, req StopInstanceRequest) (CommandReceipt, error) {
	return CommandReceipt{
		CommandID: req.CommandID,
		Status:    "pending",
	}, nil
}

// GetCommandStatus always returns "pending" for any command ID.
func (n NoopCommandDispatcher) GetCommandStatus(_ context.Context, _ string) (string, error) {
	return "pending", nil
}

// instanceCommandID builds an idempotent command identifier scoped to an instance,
// operation type, and generation.
func instanceCommandID(instanceID string, typ InstanceCommandType, generation int) string {
	return fmt.Sprintf("%s-%s-%d", instanceID, string(typ), generation)
}

// newOperationID creates a new unique operation identifier for a deploy/scale operation.
func newOperationID() string {
	return uuid.NewString()
}
