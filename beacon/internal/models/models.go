package models

import (
	"time"
)

type Server struct {
	ID        string
	Name      string
	NodeID    string
	Status    ServerStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	// ... other fields
}

type ServerStatus string

const (
	ServerStatusStarting ServerStatus = "starting"
	ServerStatusRunning  ServerStatus = "running"
	ServerStatusStopping ServerStatus = "stopping"
	ServerStatusStopped  ServerStatus = "stopped"
	ServerStatusCrashed  ServerStatus = "crashed"
)
