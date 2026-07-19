package store

import (
	"time"
)

// Placement Reservation types
type PlacementReservationStatus string

type PlacementReservationType string

type CreatePlacementReservationRequest struct {
	ServerID        string
	NodeID          string
	MigrationID     string
	ReservationType PlacementReservationType
	Status          PlacementReservationStatus
	ReservedBy      string
	CPU             int
	Memory          int64
	Disk            int64
	ExpiresAt       time.Time
}

type PlacementReservation struct {
	ID              string                     `json:"id"`
	ServerID        *string                    `json:"serverId,omitempty"`
	NodeID          string                     `json:"nodeId"`
	MigrationID     *string                    `json:"migrationId,omitempty"`
	ReservationType string                     `json:"reservationType"`
	Status          PlacementReservationStatus `json:"status"`
	ReservedBy      string                     `json:"reservedBy"`
	CPU             int                        `json:"cpu"`
	Memory          int64                      `json:"memory"`
	Disk            int64                      `json:"disk"`
	ExpiresAt       time.Time                  `json:"expiresAt"`
	ConfirmedAt     *time.Time                 `json:"confirmedAt,omitempty"`
	CancelledAt     *time.Time                 `json:"cancelledAt,omitempty"`
	ExpiredAt       *time.Time                 `json:"expiredAt,omitempty"`
	UsedAt          *time.Time                 `json:"usedAt,omitempty"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
}

// Reserved Capacity type
type ReservedCapacity struct {
	NodeID   string `json:"nodeId"`
	CPU      int    `json:"cpu"`
	Memory   int    `json:"memory"`
	Disk     int    `json:"disk"`
	Reserved int    `json:"reserved"`
}
