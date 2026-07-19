package store

import (
	"time"
)

type AuditEvent struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   *string   `json:"targetId,omitempty"`
	Metadata   string    `json:"metadata"`
	CreatedAt  time.Time `json:"createdAt"`
	ActorEmail *string   `json:"actorEmail,omitempty"`
}
