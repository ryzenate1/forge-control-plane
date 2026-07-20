package protocol

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const CurrentVersion = 1

type Command struct {
	ID                string          `json:"id"`
	IdempotencyKey    string          `json:"idempotencyKey"`
	Type              string          `json:"type"`
	ProtocolVersion   int             `json:"protocolVersion"`
	ServerID          string          `json:"serverId"`
	WorkloadID        string          `json:"workloadId,omitempty"`
	OperationID       string          `json:"operationId,omitempty"`
	DesiredGeneration int64           `json:"desiredGeneration"`
	Payload           json.RawMessage `json:"payload"`
	IssuedAt          time.Time       `json:"issuedAt"`
}

type Acknowledgement struct {
	CommandID       string          `json:"commandId"`
	ProtocolVersion int             `json:"protocolVersion"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	AcknowledgedAt  time.Time       `json:"acknowledgedAt"`
}

type Observation struct {
	WorkloadID string          `json:"workloadId"`
	InstanceID string          `json:"instanceId,omitempty"`
	Generation int64           `json:"generation"`
	State      string          `json:"state"`
	Details    json.RawMessage `json:"details,omitempty"`
	ObservedAt time.Time       `json:"observedAt"`
}

func (o Observation) Validate() error {
	if strings.TrimSpace(o.WorkloadID) == "" || strings.TrimSpace(o.State) == "" {
		return errors.New("workload id and observed state are required")
	}
	if o.Generation < 0 {
		return errors.New("observation generation must not be negative")
	}
	if len(o.Details) > 0 && !json.Valid(o.Details) {
		return errors.New("observation details must be valid JSON")
	}
	return nil
}

func (c Command) Validate() error {
	if c.ProtocolVersion != CurrentVersion {
		return errors.New("unsupported command protocol version")
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.IdempotencyKey) == "" || strings.TrimSpace(c.Type) == "" || strings.TrimSpace(c.ServerID) == "" {
		return errors.New("command id, idempotency key, type, and server id are required")
	}
	if c.DesiredGeneration < 1 {
		return errors.New("desired generation must be positive")
	}
	if len(c.Payload) > 0 && !json.Valid(c.Payload) {
		return errors.New("command payload must be valid JSON")
	}
	return nil
}

func (a Acknowledgement) Validate() error {
	if a.ProtocolVersion != CurrentVersion {
		return errors.New("unsupported acknowledgement protocol version")
	}
	if strings.TrimSpace(a.CommandID) == "" {
		return errors.New("command id is required")
	}
	switch a.Status {
	case "acknowledged", "succeeded", "failed", "cancelled":
	default:
		return errors.New("invalid acknowledgement status")
	}
	if len(a.Result) > 0 && !json.Valid(a.Result) {
		return errors.New("acknowledgement result must be valid JSON")
	}
	return nil
}
