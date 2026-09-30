package pipeline

import (
	"fmt"
	"time"
)

// Action enumerates the stage actions the executor understands.
type Action string

const (
	ActionPull        Action = "pull"
	ActionBuild       Action = "build"
	ActionDeploy      Action = "deploy"
	ActionCompose     Action = "compose"
	ActionHealthCheck Action = "health_check"
	ActionNotify      Action = "notify"
	ActionApproval    Action = "approval"
	ActionSleep       Action = "sleep"
	ActionScript      Action = "script"
)

// AllActions is the ordered action list surfaced by the def editor UI.
var AllActions = []Action{
	ActionPull,
	ActionBuild,
	ActionDeploy,
	ActionCompose,
	ActionHealthCheck,
	ActionNotify,
	ActionApproval,
	ActionSleep,
	ActionScript,
}

// Run and stage lifecycle states.
const (
	StatusQueued          = "queued"
	StatusRunning         = "running"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusCancelled       = "cancelled"
	StatusAwaitApproval   = "awaiting_approval"

	StageStatusQueued     = "queued"
	StageStatusRunning    = "running"
	StageStatusSucceeded  = "succeeded"
	StageStatusFailed     = "failed"
	StageStatusSkipped    = "skipped"
	StageStatusCancelled  = "cancelled"
	StageStatusAwaiting   = "awaiting_approval"
)

// RetryPolicy describes per-stage retry behaviour. MaxRetries is the number
// of re-attempts *after* the first try; BackoffMs is the initial sleep between
// attempts and is doubled each attempt until MaxSleepMs (0 = no cap).
type RetryPolicy struct {
	MaxRetries int `json:"maxRetries"`
	BackoffMs  int `json:"backoffMs"`
	MaxSleepMs int `json:"maxSleepMs"`
}

// Trigger describes how a pipeline may be started. Type is one of manual,
// webhook or schedule. Only the cron field is interpreted by the scheduler
// loop; webhook events are accepted via the authenticated run endpoint.
type Trigger struct {
	Type    string `json:"type"`
	Cron    string `json:"cron,omitempty"`
	Enabled bool   `json:"enabled"`
}

// StageDef is a single ordered stage inside a pipeline definition.
type StageDef struct {
	Name               string     `json:"name"`
	Action             Action     `json:"action"`
	Config             map[string]any `json:"config,omitempty"`
	TimeoutSec         int        `json:"timeoutSec"`
	Retry              RetryPolicy `json:"retry,omitempty"`
	ContinueOnFailure  bool       `json:"continueOnFailure"`
}

// Definition is the reusable pipeline template managed through the API.
type Definition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Categories  []string  `json:"categories,omitempty"`
	Stages      []StageDef `json:"stages"`
	Trigger     Trigger   `json:"trigger"`
	CreatedBy   string    `json:"createdBy,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Run is one execution of a definition. stage_snapshot is frozen at
// creation. RetryOf/RetryCount expose retry lineage in run listings.
type Run struct {
	ID              string     `json:"id"`
	PipelineID      string     `json:"pipelineId"`
	PipelineName    string     `json:"pipelineName,omitempty"`
	Trigger         string     `json:"trigger"`
	Status          string     `json:"status"`
	ProgressPct     int        `json:"progressPct"`
	CurrentStage    string     `json:"currentStage"`
	Error           string     `json:"error,omitempty"`
	RetryOf         *string    `json:"retryOf,omitempty"`
	RetryCount      int        `json:"retryCount"`
	CancelRequested bool       `json:"cancelRequested"`
	RequestedBy     string     `json:"requestedBy,omitempty"`
	QueuedAt        time.Time  `json:"queuedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	Stages          []StageRun `json:"stages,omitempty"`
}

// StageRun tracks one stage within a run execution.
type StageRun struct {
	ID                string    `json:"id"`
	RunID             string    `json:"runId"`
	PipelineID        string    `json:"pipelineId"`
	Position          int       `json:"position"`
	Name              string    `json:"name"`
	Action            string    `json:"action"`
	Config            map[string]any `json:"config,omitempty"`
	Status            string    `json:"status"`
	Attempts          int       `json:"attempts"`
	ContinueOnFailure bool      `json:"continueOnFailure"`
	TimeoutSec        int       `json:"timeoutSec,omitempty"`
	RetryPolicy       RetryPolicy `json:"retry,omitempty"`
	Error             string    `json:"error,omitempty"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// LogEntry is a single streamed pipeline log line.
type LogEntry struct {
	ID        int64  `json:"id"`
	RunID     string `json:"runId"`
	StageID   string `json:"stageId,omitempty"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt time.Time `json:"timestamp"`
}

// Artifact is the DB-recorded metadata of a persisted pipeline file.
type Artifact struct {
	ID           string    `json:"id"`
	RunID        string    `json:"runId"`
	StageID      string    `json:"stageId,omitempty"`
	Name         string    `json:"name"`
	RelativePath string    `json:"relativePath"`
	SizeBytes    int64     `json:"sizeBytes"`
	ContentType  string    `json:"contentType"`
	CreatedBy    string    `json:"createdBy,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ErrNotFound is returned by store methods for missing rows.
var ErrNotFound = fmt.Errorf("pipeline record not found")

// errApprovalRequired is a sentinel returned by the approval action executor,
// signalling the runner to pause the run instead of failing it.
var errApprovalRequired = fmt.Errorf("approval required")