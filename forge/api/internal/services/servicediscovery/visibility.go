package servicediscovery

import (
	"time"
)

type NetworkVisibilityView struct {
	Services       []ServiceVisibilityView  `json:"services"`
	TotalEndpoints int                      `json:"totalEndpoints"`
	HealthyCount   int                      `json:"healthyCount"`
	UnhealthyCount int                      `json:"unhealthyCount"`
	NodesCount     int                      `json:"nodesCount"`
	LastUpdated    time.Time                `json:"lastUpdated"`
}

type ServiceVisibilityView struct {
	ServiceName  string               `json:"serviceName"`
	ServiceID    string               `json:"serviceId"`
	TenantID     string               `json:"tenantId,omitempty"`
	EndpointCount int                 `json:"endpointCount"`
	HealthyCount int                  `json:"healthyCount"`
	Nodes        []string             `json:"nodes"`
	Access       NetworkAccess        `json:"access"`
	Endpoints    []EndpointVisibility `json:"endpoints"`
}

type EndpointVisibility struct {
	ID           string           `json:"id"`
	NodeID       string           `json:"nodeId"`
	NodeName     string           `json:"nodeName"`
	Address      string           `json:"address"`
	Port         int              `json:"port"`
	Protocol     EndpointProtocol `json:"protocol"`
	Status       EndpointStatus   `json:"status"`
	ReplicaIndex int              `json:"replicaIndex"`
	Reachable    bool             `json:"reachable,omitempty"`
	Network      NetworkAccess    `json:"network"`
	LastSeen     time.Time        `json:"lastSeen"`
}

type NodeNetworkView struct {
	NodeID     string                   `json:"nodeId"`
	NodeName   string                   `json:"nodeName"`
	Services   []string                 `json:"services"`
	Endpoints  []EndpointVisibility     `json:"endpoints"`
	Reachability []ReachabilityResult   `json:"reachability,omitempty"`
}

func BuildNetworkVisibility(registry *Registry, policy *PrivateNetworkPolicy) NetworkVisibilityView {
	services := registry.ListServices()
	view := NetworkVisibilityView{
		Services:    make([]ServiceVisibilityView, 0, len(services)),
		LastUpdated: time.Now(),
	}

	nodeSet := make(map[string]struct{})

	for _, svc := range services {
		svcView := ServiceVisibilityView{
			ServiceName:  svc.ServiceName,
			ServiceID:    svc.ServiceID,
			TenantID:     svc.TenantID,
			EndpointCount: len(svc.Endpoints),
			Endpoints:    make([]EndpointVisibility, 0, len(svc.Endpoints)),
		}

		nodeMap := make(map[string]struct{})

		for _, ep := range svc.Endpoints {
			nodeSet[ep.NodeID] = struct{}{}
			nodeMap[ep.NodeID] = struct{}{}

			if ep.Status == EndpointStatusHealthy {
				svcView.HealthyCount++
				view.HealthyCount++
			} else {
				view.UnhealthyCount++
			}

			access := NetworkAccessIsolated
			if policy != nil {
				access = policy.ClassifyEndpoint(ep)
			} else {
				access = classifySimple(ep.Address.String())
			}

			svcView.Endpoints = append(svcView.Endpoints, EndpointVisibility{
				ID:           ep.ID,
				NodeID:       ep.NodeID,
				NodeName:     ep.NodeName,
				Address:      ep.Address.String(),
				Port:         ep.Port,
				Protocol:     ep.Protocol,
				Status:       ep.Status,
				ReplicaIndex: ep.ReplicaIndex,
				Network:      access,
				LastSeen:     ep.LastHeartbeat,
			})
		}

		svcView.Access = svcViewEndpointsAccess(svcView.Endpoints)
		svcView.Nodes = mapKeysToSlice(nodeMap)
		view.Services = append(view.Services, svcView)
	}

	view.TotalEndpoints = registry.EndpointCount()
	view.NodesCount = len(nodeSet)
	return view
}

func BuildNodeView(registry *Registry, verifier *ReachabilityVerifier, nodeID string) NodeNetworkView {
	endpoints := registry.ListEndpoints(EndpointFilter{NodeID: nodeID})

	view := NodeNetworkView{
		NodeID:    nodeID,
		Services:  make([]string, 0),
		Endpoints: make([]EndpointVisibility, 0, len(endpoints)),
	}

	svcSet := make(map[string]struct{})

	for _, ep := range endpoints {
		svcSet[ep.ServiceName] = struct{}{}
		view.Endpoints = append(view.Endpoints, EndpointVisibility{
			ID:           ep.ID,
			NodeID:       ep.NodeID,
			NodeName:     ep.NodeName,
			Address:      ep.Address.String(),
			Port:         ep.Port,
			Protocol:     ep.Protocol,
			Status:       ep.Status,
			ReplicaIndex: ep.ReplicaIndex,
			LastSeen:     ep.LastHeartbeat,
		})
	}

	view.Services = mapKeysToSlice(svcSet)

	if verifier != nil {
		for _, ep := range endpoints {
			if result, ok := verifier.GetResult(ep.NodeID, ep.NodeID, ep.ServiceName); ok {
				view.Reachability = append(view.Reachability, *result)
			}
		}
	}

	return view
}

func svcViewEndpointsAccess(endpoints []EndpointVisibility) NetworkAccess {
	hasPublic := false
	hasPrivate := false

	for _, ep := range endpoints {
		switch ep.Network {
		case NetworkAccessPublic:
			hasPublic = true
		case NetworkAccessPrivate:
			hasPrivate = true
		}
	}

	if hasPublic && hasPrivate {
		return NetworkAccessPublic
	}
	if hasPublic {
		return NetworkAccessPublic
	}
	if hasPrivate {
		return NetworkAccessPrivate
	}
	return NetworkAccessIsolated
}

func classifySimple(addr string) NetworkAccess {
	if len(addr) == 0 {
		return NetworkAccessIsolated
	}
	if (len(addr) >= 3 && addr[:3] == "10.") ||
		(len(addr) >= 4 && addr[:4] == "172.") ||
		(len(addr) >= 8 && addr[:8] == "192.168.") {
		return NetworkAccessPrivate
	}
	if addr == "127.0.0.1" || addr == "::1" {
		return NetworkAccessIsolated
	}
	return NetworkAccessPublic
}

func mapKeysToSlice(m map[string]struct{}) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
