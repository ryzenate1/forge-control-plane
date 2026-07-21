package reconciler

import (
	"time"

	"gamepanel/forge/internal/store"
)

type ResourceKind string

const (
	ResourceKindServer       ResourceKind = "server"
	ResourceKindNode         ResourceKind = "node"
	ResourceKindMount        ResourceKind = "mount"
	ResourceKindDBHost       ResourceKind = "database_host"
	ResourceKindComposeStack ResourceKind = "compose_stack"
)

type ComposeDesiredState struct {
	ComposeYAML string `json:"composeYaml"`
	ComposeHash string `json:"composeHash"`
	Status      string `json:"status"`
}

type ComposeObservedState struct {
	Status   string                       `json:"status"`
	Services []ComposeObservedServiceInfo `json:"services,omitempty"`
}

type ComposeObservedServiceInfo struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
}

type DesiredStateSnapshot struct {
	ResourceID    string                 `json:"resourceId"`
	ResourceKind  ResourceKind           `json:"resourceKind"`
	ServerState   *store.ServerDesiredState `json:"serverState,omitempty"`
	NodeState     *store.NodeDesiredState   `json:"nodeState,omitempty"`
	ComposeState  *ComposeDesiredState      `json:"composeState,omitempty"`
	ConfigHash    string                 `json:"configHash"`
	Generation    int64                  `json:"generation"`
	TakenAt       time.Time              `json:"takenAt"`
}

type ObservedStateSnapshot struct {
	ResourceID    string                   `json:"resourceId"`
	ResourceKind  ResourceKind             `json:"resourceKind"`
	ServerState   *store.ServerActualState `json:"serverState,omitempty"`
	NodeState     *string                  `json:"nodeState,omitempty"`
	ComposeState  *ComposeObservedState    `json:"composeState,omitempty"`
	ConfigHash    string                   `json:"configHash"`
	Generation    int64                    `json:"generation"`
	TakenAt       time.Time                `json:"takenAt"`
}

type ReconcileSnapshot struct {
	Desired  DesiredStateSnapshot  `json:"desired"`
	Observed ObservedStateSnapshot `json:"observed"`
}

type ReconcileDiffType string

const (
	DiffCreate ReconcileDiffType = "create"
	DiffUpdate ReconcileDiffType = "update"
	DiffDelete ReconcileDiffType = "delete"
	DiffNoOp   ReconcileDiffType = "noop"
)

type ReconcileDiff struct {
	ResourceID   string            `json:"resourceId"`
	ResourceKind ResourceKind      `json:"resourceKind"`
	DiffType     ReconcileDiffType `json:"diffType"`
	DesiredHash  string            `json:"desiredHash"`
	ObservedHash string            `json:"observedHash"`
	Description  string            `json:"description"`
	Details      map[string]any    `json:"details,omitempty"`
}

type DriftKind string

const (
	DriftConfigMismatch   DriftKind = "config_drift"
	DriftMissingResource  DriftKind = "missing_resource"
	DriftOrphanedResource DriftKind = "orphaned_resource"
	DriftStateMismatch    DriftKind = "state_mismatch"
)

type DriftRecord struct {
	ResourceID   string            `json:"resourceId"`
	ResourceKind ResourceKind      `json:"resourceKind"`
	DriftKind    DriftKind         `json:"driftKind"`
	Desired      string            `json:"desired"`
	Observed     string            `json:"observed"`
	Severity     string            `json:"severity"`
	DetectedAt   time.Time         `json:"detectedAt"`
	Details      map[string]any    `json:"details,omitempty"`
}

type ReconcilePlan struct {
	ID             string          `json:"id"`
	ResourceID     string          `json:"resourceId"`
	ResourceKind   ResourceKind    `json:"resourceKind"`
	Diffs          []ReconcileDiff `json:"diffs"`
	Drifts         []DriftRecord   `json:"drifts"`
	Destructive    bool            `json:"destructive"`
	Confirmed      bool            `json:"confirmed"`
	CreatedAt      time.Time       `json:"createdAt"`
	ExecutedAt     *time.Time      `json:"executedAt,omitempty"`
}

type ReconcilePlanState string

const (
	PlanPending         ReconcilePlanState = "pending"
	PlanConfirmed       ReconcilePlanState = "confirmed"
	PlanExecuting       ReconcilePlanState = "executing"
	PlanSucceeded       ReconcilePlanState = "succeeded"
	PlanFailed          ReconcilePlanState = "failed"
	PlanCancelled       ReconcilePlanState = "cancelled"
)

type ReconcilePolicy string

const (
	ReconcileManual    ReconcilePolicy = "manual"
	ReconcileInterval  ReconcilePolicy = "interval"
)

type ReconcileResult struct {
	PlanID         string              `json:"planId"`
	ResourceID     string              `json:"resourceId"`
	ResourceKind   ResourceKind        `json:"resourceKind"`
	State          ReconcilePlanState  `json:"state"`
	OperationCount int                 `json:"operationCount"`
	Error          string              `json:"error,omitempty"`
	StartedAt      time.Time           `json:"startedAt"`
	CompletedAt    *time.Time          `json:"completedAt,omitempty"`
}
