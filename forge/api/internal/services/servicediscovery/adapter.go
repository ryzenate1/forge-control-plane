package servicediscovery

import (
	"context"
	"time"
)

type NetworkAdapter interface {
	Kind() string

	ConnectNode(ctx context.Context, nodeID, endpoint string) error

	DisconnectNode(ctx context.Context, nodeID string) error

	ListConnectedNodes(ctx context.Context) ([]string, error)

	GetNodeAddress(ctx context.Context, nodeID string) (string, error)

	IsNodeReachable(ctx context.Context, nodeID string) (bool, error)
}

type AdapterHealth struct {
	Status    string        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Nodes     int           `json:"nodes"`
	Latency   time.Duration `json:"latency,omitempty"`
	Version   string        `json:"version,omitempty"`
}
