package models

import (
	"errors"
	"strings"
	"time"
)

type Server struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	NodeID    string       `json:"nodeId"`
	Status    ServerStatus `json:"status"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
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

func (s Server) Validate() error {
	if strings.TrimSpace(s.ID) == "" || strings.ContainsAny(s.ID, "/\\\x00") {
		return errors.New("invalid server ID")
	}
	if name := strings.TrimSpace(s.Name); name == "" || len(name) > 255 {
		return errors.New("server name must contain 1 to 255 characters")
	}
	if strings.TrimSpace(s.NodeID) == "" || strings.ContainsAny(s.NodeID, "/\\\x00") {
		return errors.New("invalid node ID")
	}
	switch s.Status {
	case ServerStatusStarting, ServerStatusRunning, ServerStatusStopping, ServerStatusStopped, ServerStatusCrashed:
		return nil
	default:
		return errors.New("invalid server status")
	}
}
