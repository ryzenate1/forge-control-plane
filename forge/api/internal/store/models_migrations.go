package store

import (
	"time"
)

// Evacuation types
type EvacuationPlanStatus string

type EvacuationPlan struct {
	ID        string               `json:"id"`
	NodeID    string               `json:"nodeId"`
	Status    EvacuationPlanStatus `json:"status"`
	Items     []EvacuationItem     `json:"items"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type EvacuationItem struct {
	ID           string  `json:"id"`
	PlanID       string  `json:"planId"`
	ServerID     string  `json:"serverId"`
	SourceNodeID string  `json:"sourceNodeId"`
	TargetNodeID string  `json:"targetNodeId,omitempty"`
	Eligible     bool    `json:"eligible"`
	Reason       string  `json:"reason"`
	MigrationID  string  `json:"migrationId,omitempty"`
	Status       string  `json:"status"`
	Error        *string `json:"error,omitempty"`
}

// Migration types
type CreateMigrationRequest struct {
	ServerID       string
	SourceNodeID   string
	TargetNodeID   string
	InitiatedBy    string
	Priority       int
	TransferMethod string
}

type Migration struct {
	ID              string             `json:"id"`
	ServerID        string             `json:"serverId"`
	SourceNodeID    string             `json:"sourceNodeId"`
	TargetNodeID    string             `json:"targetNodeId"`
	Status          string             `json:"status"`
	InitiatedBy     string             `json:"initiatedBy"`
	Priority        int                `json:"priority"`
	TransferMethod  string             `json:"transferMethod"`
	FailureReason   *string            `json:"failureReason,omitempty"`
	TransferPhase   string             `json:"transferPhase,omitempty"`
	IdempotencyKey  string             `json:"idempotencyKey,omitempty"`
	ArchiveSize     int64              `json:"archiveSize,omitempty"`
	ArchiveChecksum string             `json:"archiveChecksum,omitempty"`
	CleanupPending  bool               `json:"cleanupPending,omitempty"`
	History         []MigrationHistory `json:"history,omitempty"`
	StartedAt       *time.Time         `json:"startedAt,omitempty"`
	CompletedAt     *time.Time         `json:"completedAt,omitempty"`
	Error           *string            `json:"error,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// Migration Status
type MigrationStatus string

type MigrationRun struct {
	MigrationID               string
	ProtocolVersion           string
	Phase                     string
	IdempotencyKey            string
	Attempt                   int
	LeaseOwner                string
	TargetAllocationID        string
	ArchiveSize               int64
	ArchiveChecksum           string
	SourceCredentialHash      string
	DestinationCredentialHash string
	CredentialExpiresAt       *time.Time
	CleanupPending            bool
	LastError                 string
}

type MigrationHistory struct {
	ID           string          `json:"id"`
	MigrationID  string          `json:"migrationId"`
	ServerID     string          `json:"serverId"`
	SourceNodeID string          `json:"sourceNodeId"`
	TargetNodeID string          `json:"targetNodeId"`
	Status       MigrationStatus `json:"status"`
	FromStatus   string          `json:"fromStatus"`
	ToStatus     string          `json:"toStatus"`
	Reason       string          `json:"reason"`
	StartedAt    *time.Time      `json:"startedAt,omitempty"`
	CompletedAt  *time.Time      `json:"completedAt,omitempty"`
	Error        *string         `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Observability/Timeline types
type CreateTimelineEventRequest struct {
	EventID       string
	ResourceType  string
	ResourceID    string
	EventType     string
	CorrelationID string
	Source        string
	Payload       map[string]any
	Timestamp     time.Time
}

type TimelineEvent struct {
	ID            string         `json:"id"`
	EventID       string         `json:"eventId,omitempty"`
	ResourceType  string         `json:"resourceType"`
	ResourceID    string         `json:"resourceId"`
	EventType     string         `json:"eventType"`
	CorrelationID string         `json:"correlationId"`
	Source        string         `json:"source"`
	Payload       map[string]any `json:"payload"`
	Timestamp     time.Time      `json:"timestamp"`
}

type TimelineQuery struct {
	ResourceType  string
	ResourceID    string
	CorrelationID string
	Limit         int
}
