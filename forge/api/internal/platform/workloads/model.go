package workloads

import (
	"encoding/json"
	"errors"
	"time"
)

type DesiredState string
type ObservedState string

type Workload struct {
	ID                 string        `json:"id"`
	EnvironmentID      string        `json:"environmentId"`
	Kind               Kind          `json:"kind"`
	Name               string        `json:"name"`
	DesiredGeneration  int64         `json:"desiredGeneration"`
	ObservedGeneration int64         `json:"observedGeneration"`
	DesiredState       DesiredState  `json:"desiredState"`
	ObservedState      ObservedState `json:"observedState"`
	CurrentRevisionID  string        `json:"currentRevisionId,omitempty"`
	LastObservationAt  *time.Time    `json:"lastObservationAt,omitempty"`
	LastReconcileError string        `json:"lastReconcileError,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

func (w Workload) Validate() error {
	if w.ID == "" || w.EnvironmentID == "" || w.Name == "" {
		return errors.New("workload id, environment id, and name are required")
	}
	return w.Kind.Validate()
}

type Revision struct {
	ID            string          `json:"id"`
	WorkloadID    string          `json:"workloadId"`
	Number        int64           `json:"number"`
	SchemaVersion int             `json:"schemaVersion"`
	Spec          json.RawMessage `json:"spec"`
	CreatedBy     string          `json:"createdBy,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type Instance struct {
	ID            string        `json:"id"`
	WorkloadID    string        `json:"workloadId"`
	RevisionID    string        `json:"revisionId"`
	NodeID        string        `json:"nodeId"`
	DesiredState  DesiredState  `json:"desiredState"`
	ObservedState ObservedState `json:"observedState"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}

// Route, VolumeAttachment, SecretReference, and BackupArtifact are shared
// attachment contracts. Capability modules supply their own semantics while
// the platform keeps ownership and lifecycle relationships consistent.
type Route struct {
	ID         string `json:"id"`
	WorkloadID string `json:"workloadId"`
	Host       string `json:"host"`
	Path       string `json:"path,omitempty"`
	TargetPort int    `json:"targetPort"`
	TLS        bool   `json:"tls"`
}

type VolumeAttachment struct {
	ID         string `json:"id"`
	WorkloadID string `json:"workloadId"`
	VolumeID   string `json:"volumeId"`
	MountPath  string `json:"mountPath"`
	ReadOnly   bool   `json:"readOnly"`
}

type SecretReference struct {
	ID         string `json:"id"`
	WorkloadID string `json:"workloadId"`
	SecretID   string `json:"secretId"`
	TargetKey  string `json:"targetKey"`
}

type BackupArtifact struct {
	ID         string    `json:"id"`
	WorkloadID string    `json:"workloadId"`
	Location   string    `json:"location"`
	Checksum   string    `json:"checksum"`
	CreatedAt  time.Time `json:"createdAt"`
}
