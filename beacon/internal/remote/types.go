package remote

import (
	"encoding/json"
	"time"
)

// ServerConfigurationResponse holds server config from panel
type ServerConfigurationResponse struct {
	Settings             json.RawMessage       `json:"settings"`
	ProcessConfiguration *ProcessConfiguration `json:"process_configuration"`
	Mounts               []Mount               `json:"mounts"`
}

// Mount describes a panel-approved bind mount for a server workload.
type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// RawServerData is raw server response
type RawServerData struct {
	Uuid                 string          `json:"uuid"`
	Settings             json.RawMessage `json:"settings"`
	ProcessConfiguration json.RawMessage `json:"process_configuration"`
	Suspended            *bool           `json:"suspended,omitempty"`
	Installing           *bool           `json:"is_installing,omitempty"`
	Installed            *bool           `json:"installed,omitempty"`
	Status               string          `json:"status,omitempty"`
}

// ProcessConfiguration defines process settings
type ProcessConfiguration struct {
	Startup struct {
		Done      []string `json:"done"`
		StripAnsi bool     `json:"strip_ansi"`
	} `json:"startup"`
	Stop struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"stop"`
}

type BackupPart struct {
	ETag       string `json:"etag"`
	PartNumber int    `json:"part_number"`
	Size       int64  `json:"size,omitempty"`
}

type BackupRequest struct {
	UUID         string       `json:"uuid,omitempty"`
	Checksum     string       `json:"checksum"`
	ChecksumType string       `json:"checksum_type"`
	Size         int64        `json:"size"`
	Successful   bool         `json:"successful"`
	Parts        []BackupPart `json:"parts,omitempty"`
}

// Activity represents an activity log
type Activity struct {
	ID        int                    `json:"id"`
	Event     string                 `json:"action"`
	User      string                 `json:"user"`
	Server    string                 `json:"server"`
	IP        string                 `json:"ip"`
	Timestamp string                 `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ServerStats represents server resource usage
type ServerStats struct {
	State       string  `json:"state"`
	Memory      uint64  `json:"memory_bytes"`
	MemoryLimit uint64  `json:"memory_limit_bytes"`
	CpuAbsolute float64 `json:"cpu_absolute"`
	NetworkRx   uint64  `json:"network_rx_bytes"`
	NetworkTx   uint64  `json:"network_tx_bytes"`
	Disk        int64   `json:"disk_bytes"`
	Uptime      int64   `json:"uptime_ms"`
}

// NodeHeartbeat represents node health report
type NodeHeartbeat struct {
	Version         string  `json:"version"`
	OS              string  `json:"os"`
	Architecture    string  `json:"architecture"`
	CPUThreads      int     `json:"cpuThreads"`
	MemoryMB        int64   `json:"memoryMb"`
	DiskMB          int64   `json:"diskMb"`
	DockerStatus    string  `json:"dockerStatus,omitempty"`
	RuntimeStatus   string  `json:"runtimeStatus"`
	RuntimeProvider string  `json:"runtimeProvider"`
	Error           string  `json:"error,omitempty"`
	Uptime          int64   `json:"uptime_seconds"`
	LoadAverage     float64 `json:"load_average"`
}

// PlacementReservationRequest creates a reservation
type PlacementReservationRequest struct {
	NodeID          string    `json:"nodeId"`
	ServerID        string    `json:"serverId,omitempty"`
	MigrationID     string    `json:"migrationId,omitempty"`
	ReservationType string    `json:"reservationType"`
	CPU             int       `json:"cpu"`
	Memory          int64     `json:"memory"`
	Disk            int64     `json:"disk"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// PlacementReservation represents a resource reservation
type PlacementReservation struct {
	ID              string    `json:"id"`
	NodeID          string    `json:"nodeId"`
	ServerID        *string   `json:"serverId,omitempty"`
	MigrationID     *string   `json:"migrationId,omitempty"`
	ReservationType string    `json:"reservationType"`
	CPU             int       `json:"cpu"`
	Memory          int64     `json:"memory"`
	Disk            int64     `json:"disk"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expiresAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

// EvacuationProgress reports evacuation status
type EvacuationProgress struct {
	Status           string `json:"status"`
	ServersTotal     int    `json:"serversTotal"`
	ServersCompleted int    `json:"serversCompleted"`
	ServersFailed    int    `json:"serversFailed"`
	CurrentServer    string `json:"currentServer,omitempty"`
	Error            string `json:"error,omitempty"`
}
