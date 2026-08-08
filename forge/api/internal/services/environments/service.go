package environments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

type envStore interface {
	ListInfraEndpoints(ctx context.Context) ([]store.InfraEndpoint, error)
	GetInfraEndpoint(ctx context.Context, id string) (store.InfraEndpoint, error)
	CreateInfraEndpoint(ctx context.Context, req domain.CreateEndpointRequest, actorID *string) (store.InfraEndpoint, error)
	UpdateInfraEndpoint(ctx context.Context, id string, req domain.UpdateEndpointRequest, actorID *string) (store.InfraEndpoint, error)
	DeleteInfraEndpoint(ctx context.Context, id string, actorID *string) error
	ListInfraEndpointNodes(ctx context.Context, endpointID string) ([]store.InfraEndpointNode, error)
	AddNodeToInfraEndpoint(ctx context.Context, endpointID, nodeID string) error
	RemoveNodeFromInfraEndpoint(ctx context.Context, endpointID, nodeID string) error
	ListInfraEndpointAccessPolicies(ctx context.Context, endpointID string) ([]store.InfraEndpointAccessPolicy, error)
	SetInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID, role string) (store.InfraEndpointAccessPolicy, error)
	RemoveInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID string) error
	RecordEndpointHealth(ctx context.Context, endpointID string, status string, reachable bool, score float64, version string, containers, images, volumes int, errMsg string) error
	ListEndpointHealthHistory(ctx context.Context, endpointID string, limit int) ([]store.InfraEndpointHealthHistory, error)
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
}

type Service struct {
	store envStore
}

func New(st envStore) *Service {
	return &Service{store: st}
}

func (svc *Service) List(ctx context.Context) ([]domain.Endpoint, error) {
	endpoints, err := svc.store.ListInfraEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		result = append(result, toDomainEndpoint(ep))
	}
	return result, nil
}

func (svc *Service) Get(ctx context.Context, id string) (domain.Endpoint, error) {
	ep, err := svc.store.GetInfraEndpoint(ctx, id)
	if err != nil {
		return domain.Endpoint{}, err
	}
	return toDomainEndpoint(ep), nil
}

func (svc *Service) Create(ctx context.Context, req domain.CreateEndpointRequest, actorID *string) (domain.Endpoint, error) {
	if strings.TrimSpace(req.Name) == "" {
		return domain.Endpoint{}, errors.New("name is required")
	}
	ep, err := svc.store.CreateInfraEndpoint(ctx, req, actorID)
	if err != nil {
		return domain.Endpoint{}, err
	}
	return toDomainEndpoint(ep), nil
}

func (svc *Service) Update(ctx context.Context, id string, req domain.UpdateEndpointRequest, actorID *string) (domain.Endpoint, error) {
	ep, err := svc.store.UpdateInfraEndpoint(ctx, id, req, actorID)
	if err != nil {
		return domain.Endpoint{}, err
	}
	return toDomainEndpoint(ep), nil
}

func (svc *Service) Delete(ctx context.Context, id string, actorID *string) error {
	return svc.store.DeleteInfraEndpoint(ctx, id, actorID)
}

func (svc *Service) AddNode(ctx context.Context, endpointID, nodeID string) error {
	return svc.store.AddNodeToInfraEndpoint(ctx, endpointID, nodeID)
}

func (svc *Service) RemoveNode(ctx context.Context, endpointID, nodeID string) error {
	return svc.store.RemoveNodeFromInfraEndpoint(ctx, endpointID, nodeID)
}

func (svc *Service) SetAccessPolicy(ctx context.Context, endpointID, principalType, principalID, role string) (domain.AccessPolicy, error) {
	p, err := svc.store.SetInfraEndpointAccessPolicy(ctx, endpointID, principalType, principalID, role)
	if err != nil {
		return domain.AccessPolicy{}, err
	}
	return domain.AccessPolicy{
		ID:            p.ID,
		EndpointID:    p.EndpointID,
		PrincipalType: p.PrincipalType,
		PrincipalID:   p.PrincipalID,
		Role:          p.Role,
		CreatedAt:     p.CreatedAt,
	}, nil
}

func (svc *Service) RemoveAccessPolicy(ctx context.Context, endpointID, principalType, principalID string) error {
	return svc.store.RemoveInfraEndpointAccessPolicy(ctx, endpointID, principalType, principalID)
}

func (svc *Service) ListAccessPolicies(ctx context.Context, endpointID string) ([]domain.AccessPolicy, error) {
	policies, err := svc.store.ListInfraEndpointAccessPolicies(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.AccessPolicy, 0, len(policies))
	for _, p := range policies {
		result = append(result, domain.AccessPolicy{
			ID:            p.ID,
			EndpointID:    p.EndpointID,
			PrincipalType: p.PrincipalType,
			PrincipalID:   p.PrincipalID,
			Role:          p.Role,
			CreatedAt:     p.CreatedAt,
		})
	}
	return result, nil
}

func (svc *Service) Diagnostics(ctx context.Context, endpointID string) (domain.EndpointDiagnostics, error) {
	ep, err := svc.store.GetInfraEndpoint(ctx, endpointID)
	if err != nil {
		return domain.EndpointDiagnostics{}, err
	}

	nodes, err := svc.store.ListInfraEndpointNodes(ctx, endpointID)
	if err != nil {
		return domain.EndpointDiagnostics{}, err
	}

	diag := domain.EndpointDiagnostics{
		EndpointID: ep.ID,
		Reachable:  ep.Reachable,
		Version:    ep.Version,
		CheckedAt:  time.Now().UTC(),
	}

	for _, en := range nodes {
		node, err := svc.store.GetNode(ctx, en.NodeID)
		if err != nil {
			continue
		}
		servers, err := svc.store.ListServersForNode(ctx, en.NodeID)
		if err != nil {
			continue
		}

		totalMem := 0
		totalCPU := 0
		totalDisk := 0
		for _, s := range servers {
			totalMem += s.MemoryMB
			totalCPU += s.CPUShares
			totalDisk += s.DiskMB
		}

		diag.Nodes = append(diag.Nodes, domain.NodeSummary{
			NodeID:       node.ID,
			Name:         node.Name,
			Status:       node.Status,
			ServerCount:  len(servers),
			AllocatedMem: totalMem,
			AllocatedCPU: totalCPU,
			AllocatedDisk: totalDisk,
		})
		if node.NodeMemoryMB != nil {
			diag.TotalMemoryMB += int64(*node.NodeMemoryMB)
		}
		if node.NodeDiskMB != nil {
			diag.TotalDiskMB += int64(*node.NodeDiskMB)
		}
		diag.UsedMemoryMB += int64(totalMem)
		diag.UsedDiskMB += int64(totalDisk)
	}

	return diag, nil
}

func (svc *Service) Inventory(ctx context.Context, endpointID string) (domain.InventorySummary, error) {
	diag, err := svc.Diagnostics(ctx, endpointID)
	if err != nil {
		return domain.InventorySummary{}, err
	}

	summary := domain.InventorySummary{
		TotalServers:   0,
		TotalContainers: 0,
		TotalImages:    0,
		TotalVolumes:   0,
		UsedMemoryMB:   diag.UsedMemoryMB,
		TotalMemoryMB:  diag.TotalMemoryMB,
		UsedDiskMB:     diag.UsedDiskMB,
		TotalDiskMB:    diag.TotalDiskMB,
	}

	for _, n := range diag.Nodes {
		summary.TotalServers += n.ServerCount
	}

	return summary, nil
}

func (svc *Service) RecordHealth(ctx context.Context, endpointID string, status string, reachable bool, score float64, version string, containers, images, volumes int, errMsg string) error {
	return svc.store.RecordEndpointHealth(ctx, endpointID, status, reachable, score, version, containers, images, volumes, errMsg)
}

func (svc *Service) HealthHistory(ctx context.Context, endpointID string, limit int) ([]domain.HealthRecord, error) {
	records, err := svc.store.ListEndpointHealthHistory(ctx, endpointID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.HealthRecord, 0, len(records))
	for _, r := range records {
		result = append(result, domain.HealthRecord{
			ID:          r.ID,
			EndpointID:  r.EndpointID,
			Status:      domain.EndpointStatus(r.Status),
			Reachable:   r.Reachable,
			HealthScore: r.HealthScore,
			Version:     r.Version,
			Containers:  r.TotalContainers,
			Images:      r.TotalImages,
			Volumes:     r.TotalVolumes,
			Error:       r.ErrorMessage,
			ObservedAt:  r.ObservedAt,
		})
	}
	return result, nil
}

func (svc *Service) CheckEndpointAccess(ctx context.Context, endpointID, userID, globalRole string, orgRoles []string) error {
	if globalRole == "admin" {
		return nil
	}
	policies, err := svc.store.ListInfraEndpointAccessPolicies(ctx, endpointID)
	if err != nil {
		return err
	}
	for _, p := range policies {
		if p.PrincipalType == "user" && p.PrincipalID == userID {
			return nil
		}
		if p.PrincipalType == "org" {
			for _, orgRole := range orgRoles {
				if orgRole == p.PrincipalID || strings.HasPrefix(p.PrincipalID, orgRole) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("access denied to endpoint %s", endpointID)
}

func toDomainEndpoint(ep store.InfraEndpoint) domain.Endpoint {
	return domain.Endpoint{
		ID:              ep.ID,
		Name:            ep.Name,
		Description:     ep.Description,
		EndpointType:    domain.EndpointType(ep.EndpointType),
		ConnectionMode:  domain.ConnectionMode(ep.ConnectionMode),
		Status:          domain.EndpointStatus(ep.Status),
		EdgeID:          ep.EdgeID,
		Tags:            ep.Tags,
		Labels:          make([]domain.LabelPair, len(ep.Labels)),
		URL:             ep.URL,
		ProjectID:       ep.ProjectID,
		GroupID:         ep.GroupID,
		Reachable:       ep.Reachable,
		Version:         ep.Version,
		TotalContainers: ep.TotalContainers,
		TotalImages:     ep.TotalImages,
		TotalVolumes:    ep.TotalVolumes,
		CreatedAt:       ep.CreatedAt,
		UpdatedAt:       ep.UpdatedAt,
	}
}
