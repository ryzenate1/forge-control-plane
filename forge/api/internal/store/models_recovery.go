package store

import (
	"time"
)

// Recovery Plan types
type RecoveryPlanStatus string

type RecoveryPlan struct {
	ID        string             `json:"id"`
	NodeID    string             `json:"nodeId"`
	Status    RecoveryPlanStatus `json:"status"`
	Reason    string             `json:"reason"`
	Items     []RecoveryItem     `json:"items"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type RecoveryStep struct {
	ID          string     `json:"id"`
	PlanID      string     `json:"planId"`
	StepType    string     `json:"stepType"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	ExecutedAt  *time.Time `json:"executedAt,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

// Recovery Item types
type RecoveryItemStatus string

type RecoveryItem struct {
	ID                   string    `json:"id"`
	PlanID               string    `json:"planId"`
	ServerID             string    `json:"serverId"`
	SourceNodeID         string    `json:"sourceNodeId"`
	TargetNodeID         string    `json:"targetNodeId,omitempty"`
	ReservationID        string    `json:"reservationId,omitempty"`
	MigrationID          string    `json:"migrationId,omitempty"`
	SourceBackupName     string    `json:"sourceBackupName,omitempty"`
	SourceBackupChecksum string    `json:"sourceBackupChecksum,omitempty"`
	SourceBackupSize     int64     `json:"sourceBackupSize,omitempty"`
	Status               string    `json:"status"`
	Reason               string    `json:"reason,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}
