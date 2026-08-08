package servicediscovery

import (
	"net/netip"
	"time"
)

type EndpointStatus string

const (
	EndpointStatusHealthy   EndpointStatus = "healthy"
	EndpointStatusUnhealthy EndpointStatus = "unhealthy"
	EndpointStatusUnknown   EndpointStatus = "unknown"
	EndpointStatusDraining  EndpointStatus = "draining"
)

type EndpointProtocol string

const (
	ProtocolTCP EndpointProtocol = "tcp"
	ProtocolUDP EndpointProtocol = "udp"
)

type ServiceEndpoint struct {
	ID            string           `json:"id"`
	ServiceName   string           `json:"serviceName"`
	ServiceID     string           `json:"serviceId"`
	NodeID        string           `json:"nodeId"`
	NodeName      string           `json:"nodeName"`
	RegionID      string           `json:"regionId,omitempty"`
	Address       netip.Addr       `json:"address"`
	Port          int              `json:"port"`
	Protocol      EndpointProtocol `json:"protocol"`
	Status        EndpointStatus   `json:"status"`
	ReplicaIndex  int              `json:"replicaIndex"`
	TenantID      string           `json:"tenantId,omitempty"`
	LastHeartbeat time.Time        `json:"lastHeartbeat"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type EndpointSet struct {
	ServiceName string            `json:"serviceName"`
	ServiceID   string            `json:"serviceId"`
	Endpoints   []ServiceEndpoint `json:"endpoints"`
	TenantID    string            `json:"tenantId,omitempty"`
}

type EndpointFilter struct {
	ServiceName string         `json:"serviceName,omitempty"`
	ServiceID   string         `json:"serviceId,omitempty"`
	NodeID      string         `json:"nodeId,omitempty"`
	TenantID    string         `json:"tenantId,omitempty"`
	Status      EndpointStatus `json:"status,omitempty"`
	HealthyOnly bool           `json:"healthyOnly"`
}

type ReachabilityResult struct {
	SourceNodeID string `json:"sourceNodeId"`
	TargetNodeID string `json:"targetNodeId"`
	ServiceName  string `json:"serviceName"`
	Reachable    bool   `json:"reachable"`
	Latency      string `json:"latency,omitempty"`
	Error        string `json:"error,omitempty"`
	CheckedAt    time.Time `json:"checkedAt"`
}
