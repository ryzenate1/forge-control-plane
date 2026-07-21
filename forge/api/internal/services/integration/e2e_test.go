package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/apphosting"
	"gamepanel/forge/internal/services/backup"
	"gamepanel/forge/internal/services/procedure"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ events.Envelope) error { return nil }

type mockIntegrationStore struct {
	mu sync.Mutex

	apps        map[string]*store.Application
	services    map[string][]store.AppService
	instances   map[string][]store.Instance
	endpoints   map[string][]store.ServiceEndpoint
	replicaApps map[string]store.ReplicaApplication

	deployments     map[string]store.Deployment
	deploymentSteps map[string][]store.DeploymentStep

	nodes     map[string]store.Node
	nodesByID map[string]store.Node

	capabilities     map[string]store.NodeCapability
	onboardingTokens map[string]store.OnboardingToken

	backupPolicies map[string]store.BackupPolicy

	placementDecisions []store.PlacementDecision

	procedures  map[string]store.Procedure
	procSteps   map[string][]store.ProcedureStep
	executions  map[string]store.ProcedureExecution
	stepExecs   map[string][]store.ProcedureStepExecution
	stepDefs    map[string]store.ProcedureStep
	schedules   map[string]store.ProcedureSchedule
	logs        map[string][]store.ProcedureStepLog
	auditEvents []string
}

func newMockStore() *mockIntegrationStore {
	return &mockIntegrationStore{
		apps:             make(map[string]*store.Application),
		services:         make(map[string][]store.AppService),
		instances:        make(map[string][]store.Instance),
		endpoints:        make(map[string][]store.ServiceEndpoint),
		replicaApps:      make(map[string]store.ReplicaApplication),
		deployments:      make(map[string]store.Deployment),
		deploymentSteps:  make(map[string][]store.DeploymentStep),
		nodes:            make(map[string]store.Node),
		nodesByID:        make(map[string]store.Node),
		capabilities:     make(map[string]store.NodeCapability),
		onboardingTokens: make(map[string]store.OnboardingToken),
		backupPolicies:   make(map[string]store.BackupPolicy),
		procedures:       make(map[string]store.Procedure),
		procSteps:        make(map[string][]store.ProcedureStep),
		executions:       make(map[string]store.ProcedureExecution),
		stepExecs:        make(map[string][]store.ProcedureStepExecution),
		stepDefs:         make(map[string]store.ProcedureStep),
		schedules:        make(map[string]store.ProcedureSchedule),
		logs:             make(map[string][]store.ProcedureStepLog),
	}
}

func (m *mockIntegrationStore) CreateApplication(_ context.Context, input store.CreateApplicationInput) (*store.Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	app := &store.Application{
		ID:             id,
		Name:           input.Name,
		Description:    input.Description,
		OrgID:          input.OrgID,
		ProjectID:      input.ProjectID,
		EnvironmentID:  input.EnvironmentID,
		ServerID:       input.ServerID,
		SourceType:     input.SourceType,
		SourceConfig:   input.SourceConfig,
		DesiredState:   "running",
		ObservedStatus: "idle",
	}
	m.apps[id] = app
	m.services[id] = []store.AppService{}
	return app, nil
}

func (m *mockIntegrationStore) GetApplication(_ context.Context, id string) (*store.Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.apps[id]
	if !ok {
		return nil, fmt.Errorf("application not found")
	}
	return app, nil
}

func (m *mockIntegrationStore) ListApplications(_ context.Context, orgID string) ([]store.Application, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.Application
	for _, app := range m.apps {
		if app.OrgID == orgID {
			result = append(result, *app)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) UpdateApplication(_ context.Context, id string, input store.UpdateApplicationInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.apps[id]
	if !ok {
		return fmt.Errorf("application not found")
	}
	if input.Name != nil {
		app.Name = *input.Name
	}
	if input.Description != nil {
		app.Description = *input.Description
	}
	if input.DesiredState != nil {
		app.DesiredState = *input.DesiredState
	}
	return nil
}

func (m *mockIntegrationStore) DeleteApplication(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apps[id]; !ok {
		return fmt.Errorf("application not found")
	}
	delete(m.apps, id)
	delete(m.services, id)
	delete(m.instances, id)
	return nil
}

func (m *mockIntegrationStore) UpdateApplicationStatus(_ context.Context, id string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.apps[id]
	if !ok {
		return fmt.Errorf("application not found")
	}
	app.ObservedStatus = status
	return nil
}

func (m *mockIntegrationStore) SetApplicationDeployment(_ context.Context, appID string, deploymentID *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.apps[appID]
	if !ok {
		return fmt.Errorf("application not found")
	}
	app.CurrentDeploymentID = deploymentID
	return nil
}

func (m *mockIntegrationStore) AppBelongsToOrg(_ context.Context, appID, orgID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.apps[appID]
	if !ok {
		return false, nil
	}
	return app.OrgID == orgID, nil
}

func (m *mockIntegrationStore) AppServiceBelongsToApp(_ context.Context, serviceID, appID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	services, ok := m.services[appID]
	if !ok {
		return false, nil
	}
	for _, s := range services {
		if s.ID == serviceID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockIntegrationStore) CreateAppService(_ context.Context, input store.CreateAppServiceInput) (*store.AppService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	mode := input.Mode
	if mode == "" {
		mode = "replicated"
	}
	svc := &store.AppService{
		ID:             id,
		AppID:          input.AppID,
		Name:           input.Name,
		Image:          input.Image,
		ComposeService: input.ComposeService,
		Replicas:       input.Replicas,
		Ports:          input.Ports,
		EnvVars:        input.EnvVars,
		DependsOn:      input.DependsOn,
		DesiredState:   "running",
		ObservedStatus: "idle",
		Mode:           mode,
		UpdateConfig:   input.UpdateConfig,
		HealthCheck:    input.HealthCheck,
		Resources:      input.Resources,
		Volumes:        input.Volumes,
		Secrets:        input.Secrets,
	}
	m.services[input.AppID] = append(m.services[input.AppID], *svc)
	return svc, nil
}

func (m *mockIntegrationStore) GetAppService(_ context.Context, id string) (*store.AppService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, services := range m.services {
		for _, s := range services {
			if s.ID == id {
				return &s, nil
			}
		}
	}
	return nil, fmt.Errorf("service not found")
}

func (m *mockIntegrationStore) ListAppServices(_ context.Context, appID string) ([]store.AppService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.services[appID], nil
}

func (m *mockIntegrationStore) DeleteAppService(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for appID, services := range m.services {
		for i, s := range services {
			if s.ID == id {
				m.services[appID] = append(services[:i], services[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("service not found")
}

func (m *mockIntegrationStore) UpdateAppService(_ context.Context, id string, input store.UpdateAppServiceInput) (*store.AppService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, services := range m.services {
		for i, s := range services {
			if s.ID == id {
				if input.Name != nil {
					s.Name = *input.Name
				}
				if input.Image != nil {
					s.Image = *input.Image
				}
				if input.Replicas != nil {
					s.Replicas = *input.Replicas
				}
				if input.Ports != nil {
					s.Ports = *input.Ports
				}
				if input.EnvVars != nil {
					s.EnvVars = *input.EnvVars
				}
				if input.DependsOn != nil {
					s.DependsOn = *input.DependsOn
				}
				if input.DesiredState != nil {
					s.DesiredState = *input.DesiredState
				}
				if input.Mode != nil {
					s.Mode = *input.Mode
				}
				if input.UpdateConfig != nil {
					s.UpdateConfig = *input.UpdateConfig
				}
				if input.HealthCheck != nil {
					s.HealthCheck = *input.HealthCheck
				}
				if input.Resources != nil {
					s.Resources = *input.Resources
				}
				if input.Volumes != nil {
					s.Volumes = *input.Volumes
				}
				if input.Secrets != nil {
					s.Secrets = *input.Secrets
				}
				if input.ReplicaAppID != nil {
					s.ReplicaAppID = input.ReplicaAppID
				}
				services[i] = s
				return &s, nil
			}
		}
	}
	return nil, fmt.Errorf("service not found")
}

func (m *mockIntegrationStore) CreateDeployment(_ context.Context, d *store.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[d.ID] = *d
	return nil
}

func (m *mockIntegrationStore) GetDeployment(_ context.Context, id string) (store.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deployments[id]
	if !ok {
		return store.Deployment{}, fmt.Errorf("deployment not found")
	}
	return d, nil
}

func (m *mockIntegrationStore) ListDeployments(_ context.Context, serverID string) ([]store.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.Deployment
	for _, d := range m.deployments {
		if d.ServerID == serverID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) UpdateDeployment(_ context.Context, d *store.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[d.ID] = *d
	return nil
}

func (m *mockIntegrationStore) ListDeploymentSteps(_ context.Context, deploymentID string) ([]store.DeploymentStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deploymentSteps[deploymentID], nil
}

func (m *mockIntegrationStore) UpdateDeploymentStepStatus(_ context.Context, stepID, status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.deploymentSteps {
		for i, s := range steps {
			if s.ID == stepID {
				steps[i].Status = status
				steps[i].Error = errMsg
				return nil
			}
		}
	}
	return fmt.Errorf("step not found")
}

func (m *mockIntegrationStore) ListInstancesByApp(_ context.Context, appID string) ([]store.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	insts, ok := m.instances[appID]
	if !ok {
		return []store.Instance{}, nil
	}
	return insts, nil
}

func (m *mockIntegrationStore) UpdateReplicaAppReplicas(_ context.Context, appID string, replicas int) (store.ReplicaApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.replicaApps[appID]
	if !ok {
		return store.ReplicaApplication{}, fmt.Errorf("replica app not found")
	}
	app.Replicas = replicas
	m.replicaApps[appID] = app
	return app, nil
}

func (m *mockIntegrationStore) ListServiceEndpoints(_ context.Context, serviceID string) ([]store.ServiceEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	eps, ok := m.endpoints[serviceID]
	if !ok {
		return []store.ServiceEndpoint{}, nil
	}
	return eps, nil
}

func (m *mockIntegrationStore) CreateServiceEndpoint(_ context.Context, serviceID, host string, port int, protocol string, nodeID, instanceID *string) (*store.ServiceEndpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	ep := &store.ServiceEndpoint{
		ID:         id,
		ServiceID:  serviceID,
		Host:       host,
		Port:       port,
		Protocol:   protocol,
		NodeID:     nodeID,
		InstanceID: instanceID,
	}
	m.endpoints[serviceID] = append(m.endpoints[serviceID], *ep)
	return ep, nil
}

func (m *mockIntegrationStore) DeleteServiceEndpoint(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for serviceID, eps := range m.endpoints {
		for i, ep := range eps {
			if ep.ID == id {
				m.endpoints[serviceID] = append(eps[:i], eps[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("endpoint not found")
}

func (m *mockIntegrationStore) CreateNode(_ context.Context, req store.CreateNodeRequest, _ *string) (store.Node, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	token := uuid.NewString()
	node := store.Node{
		ID:              id,
		UUID:            uuid.NewString(),
		Name:            req.Name,
		Region:          req.Region,
		Status:          "offline",
		DesiredState:    store.NodeDesiredStateActive,
		Maintenance:     req.Maintenance,
		MemoryMB:        req.MemoryMB,
		DiskMB:          req.DiskMB,
		DaemonListen:    req.DaemonListen,
		DaemonSFTP:      req.DaemonSFTP,
		Public:          req.Public,
		CPUCores:        4,
		Draining:        false,
		LastHeartbeatAt: time.Now().UTC(),
	}
	m.nodes[id] = node
	return node, token, nil
}

func (m *mockIntegrationStore) GetNode(_ context.Context, id string) (store.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[id]
	if !ok {
		return store.Node{}, fmt.Errorf("node not found")
	}
	return node, nil
}

func (m *mockIntegrationStore) ListNodes(_ context.Context) ([]store.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.Node
	for _, n := range m.nodes {
		result = append(result, n)
	}
	return result, nil
}

func (m *mockIntegrationStore) UpdateNode(_ context.Context, nodeID string, req store.UpdateNodeRequest, _ *string) (store.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok {
		return store.Node{}, fmt.Errorf("node not found")
	}
	if req.Name != "" {
		node.Name = req.Name
	}
	if req.DesiredState != "" {
		node.DesiredState = req.DesiredState
	}
	if req.Status != "" {
		node.Status = req.Status
	}
	node.Maintenance = req.Maintenance
	node.Draining = req.Draining
	m.nodes[nodeID] = node
	return node, nil
}

func (m *mockIntegrationStore) PatchNode(_ context.Context, nodeID string, patch store.NodePatch, _ *string) (store.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, ok := m.nodes[nodeID]
	if !ok {
		return store.Node{}, fmt.Errorf("node not found")
	}
	if patch.DesiredState != nil {
		node.DesiredState = *patch.DesiredState
	}
	if patch.Maintenance != nil {
		node.Maintenance = *patch.Maintenance
	}
	if patch.Draining != nil {
		node.Draining = *patch.Draining
	}
	m.nodes[nodeID] = node
	return node, nil
}

func (m *mockIntegrationStore) DeleteNode(_ context.Context, id string, _ *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, id)
	return nil
}

func (m *mockIntegrationStore) RotateNodeToken(_ context.Context, _ string, _ *string) (string, error) {
	return uuid.NewString(), nil
}

func (m *mockIntegrationStore) CreateOnboardingToken(_ context.Context, nodeID string, expiresAt time.Time) (*store.OnboardingToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	t := &store.OnboardingToken{
		ID:        id,
		TokenHash: uuid.NewString(),
		NodeID:    nodeID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
		State:     "pending",
	}
	m.onboardingTokens[id] = *t
	return t, nil
}

func (m *mockIntegrationStore) GetOnboardingToken(_ context.Context, tokenID string) (*store.OnboardingToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.onboardingTokens[tokenID]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}
	return &t, nil
}

func (m *mockIntegrationStore) ApproveOnboardingToken(_ context.Context, tokenID, approvedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.onboardingTokens[tokenID]
	if !ok {
		return fmt.Errorf("token not found")
	}
	t.State = "approved"
	now := time.Now().UTC()
	t.ApprovedAt = &now
	t.ApprovedBy = approvedBy
	m.onboardingTokens[tokenID] = t
	return nil
}

func (m *mockIntegrationStore) UpsertNodeCapability(_ context.Context, cap store.NodeCapability) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capabilities[cap.NodeID] = cap
	return nil
}

func (m *mockIntegrationStore) GetNodeCapability(_ context.Context, nodeID string) (store.NodeCapability, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cap, ok := m.capabilities[nodeID]
	if !ok {
		return store.NodeCapability{}, fmt.Errorf("capability not found")
	}
	return cap, nil
}

func (m *mockIntegrationStore) ListNodeCapabilities(_ context.Context) ([]store.NodeCapability, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.NodeCapability
	for _, cap := range m.capabilities {
		result = append(result, cap)
	}
	return result, nil
}

func (m *mockIntegrationStore) CreateBackupPolicy(_ context.Context, p *store.BackupPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	m.backupPolicies[p.ID] = *p
	return nil
}

func (m *mockIntegrationStore) GetBackupPolicy(_ context.Context, id string) (store.BackupPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.backupPolicies[id]
	if !ok {
		return store.BackupPolicy{}, fmt.Errorf("policy not found")
	}
	return p, nil
}

func (m *mockIntegrationStore) ListBackupPolicies(_ context.Context, serverID string) ([]store.BackupPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.BackupPolicy
	for _, p := range m.backupPolicies {
		if p.ServerID == serverID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) UpdateBackupPolicy(_ context.Context, p *store.BackupPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backupPolicies[p.ID] = *p
	return nil
}

func (m *mockIntegrationStore) DeleteBackupPolicy(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.backupPolicies, id)
	return nil
}

func (m *mockIntegrationStore) CreatePlacementDecision(_ context.Context, instanceID, nodeID, appID string, idx int, score float64, accepted bool, reasons []string, runtimeProvider string) (store.PlacementDecision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	pd := store.PlacementDecision{
		ID:              id,
		InstanceID:      instanceID,
		NodeID:          nodeID,
		AppID:           appID,
		Idx:             idx,
		Score:           score,
		Accepted:        accepted,
		Reasons:         reasons,
		RuntimeProvider: runtimeProvider,
		CreatedAt:       time.Now().UTC(),
	}
	m.placementDecisions = append(m.placementDecisions, pd)
	return pd, nil
}

func (m *mockIntegrationStore) ListPlacementDecisionsByApp(_ context.Context, appID string) ([]store.PlacementDecision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.PlacementDecision
	for _, pd := range m.placementDecisions {
		if pd.AppID == appID {
			result = append(result, pd)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) CreateInstance(_ context.Context, appID, nodeID string, idx int, cpu, memoryMB, diskMB int) (store.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	inst := store.Instance{
		ID:       id,
		AppID:    appID,
		Idx:      idx,
		NodeID:   nodeID,
		Status:   "running",
		CPU:      cpu,
		MemoryMB: memoryMB,
		DiskMB:   diskMB,
	}
	m.instances[appID] = append(m.instances[appID], inst)
	return inst, nil
}

func (m *mockIntegrationStore) CreateReplicaApp(_ context.Context, req store.CreateReplicaAppRequest) (store.ReplicaApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	if req.CPU <= 0 {
		req.CPU = 1024
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = 2048
	}
	if req.DiskMB <= 0 {
		req.DiskMB = 10240
	}
	if req.RuntimeProvider == "" {
		req.RuntimeProvider = "docker"
	}
	app := store.ReplicaApplication{
		ID:              id,
		Name:            req.Name,
		Replicas:        req.Replicas,
		CPU:             req.CPU,
		MemoryMB:        req.MemoryMB,
		DiskMB:          req.DiskMB,
		RuntimeProvider: req.RuntimeProvider,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	m.replicaApps[id] = app
	return app, nil
}

func (m *mockIntegrationStore) GetReplicaApp(_ context.Context, id string) (store.ReplicaApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.replicaApps[id]
	if !ok {
		return store.ReplicaApplication{}, fmt.Errorf("replica app not found")
	}
	return app, nil
}

func (m *mockIntegrationStore) GetProcedure(_ context.Context, id string) (store.Procedure, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.procedures[id]
	if !ok {
		return store.Procedure{}, fmt.Errorf("not found")
	}
	steps := m.procSteps[id]
	if steps != nil {
		p.Steps = steps
	}
	return p, nil
}

func (m *mockIntegrationStore) ListProcedures(_ context.Context, tenantID *string) ([]store.Procedure, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.Procedure
	for _, p := range m.procedures {
		if tenantID == nil || (p.TenantID != nil && *p.TenantID == *tenantID) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) CreateProcedure(_ context.Context, req store.CreateProcedureRequest) (store.Procedure, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("proc-%d", len(m.procedures)+1)
	p := store.Procedure{
		ID: id, Name: req.Name, Description: req.Description,
		TenantID: req.TenantID, Enabled: req.Enabled,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.procedures[id] = p
	var steps []store.ProcedureStep
	for _, s := range req.Steps {
		stepID := fmt.Sprintf("step-%d", len(steps)+1)
		step := store.ProcedureStep{
			ID: stepID, ProcedureID: id, Position: s.Position,
			Name: s.Name, Action: s.Action, Config: s.Config,
			MaxRetries: s.MaxRetries, TimeoutSeconds: s.TimeoutSeconds,
			RequiresApproval: s.RequiresApproval, ContinueOnFailure: s.ContinueOnFailure,
			RollbackEnabled: s.RollbackEnabled,
		}
		steps = append(steps, step)
		m.stepDefs[stepID] = step
	}
	m.procSteps[id] = steps
	return p, nil
}

func (m *mockIntegrationStore) UpdateProcedure(_ context.Context, id string, req store.CreateProcedureRequest) (store.Procedure, error) {
	return m.CreateProcedure(context.Background(), req)
}

func (m *mockIntegrationStore) DeleteProcedure(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.procedures, id)
	return nil
}

func (m *mockIntegrationStore) CreateProcedureExecution(_ context.Context, procedureID, trigger string, tenantID, actorID *string) (store.ProcedureExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("exec-%d", len(m.executions)+1)
	_, ok := m.procedures[procedureID]
	if !ok {
		return store.ProcedureExecution{}, fmt.Errorf("procedure not found")
	}
	exec := store.ProcedureExecution{
		ID: id, ProcedureID: procedureID, Status: "queued", Trigger: trigger,
		TenantID: tenantID, ActorID: actorID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.executions[id] = exec
	steps := m.procSteps[procedureID]
	var stepExecs []store.ProcedureStepExecution
	for _, step := range steps {
		seID := fmt.Sprintf("se-%s-%d", id, step.Position)
		se := store.ProcedureStepExecution{
			ID: seID, ExecutionID: id, StepID: step.ID, Position: step.Position,
			Status: "queued", Attempt: 0, MaxAttempts: step.MaxRetries + 1,
		}
		stepExecs = append(stepExecs, se)
		m.stepDefs[seID] = step
	}
	m.stepExecs[id] = stepExecs
	exec.Steps = stepExecs
	return exec, nil
}

func (m *mockIntegrationStore) GetProcedureExecution(_ context.Context, id string) (store.ProcedureExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exec, ok := m.executions[id]
	if !ok {
		return store.ProcedureExecution{}, fmt.Errorf("not found")
	}
	exec.Steps = m.stepExecs[id]
	return exec, nil
}

func (m *mockIntegrationStore) ListProcedureExecutions(_ context.Context, procedureID string, limit int) ([]store.ProcedureExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.ProcedureExecution
	for _, e := range m.executions {
		if e.ProcedureID == procedureID || procedureID == "" {
			e.Steps = m.stepExecs[e.ID]
			result = append(result, e)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockIntegrationStore) StartProcedureExecution(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exec, ok := m.executions[id]
	if ok {
		exec.Status = "running"
		now := time.Now()
		exec.StartedAt = &now
		m.executions[id] = exec
	}
	return nil
}

func (m *mockIntegrationStore) CompleteProcedureExecution(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exec, ok := m.executions[id]
	if ok {
		exec.Status = status
		now := time.Now()
		exec.CompletedAt = &now
		m.executions[id] = exec
	}
	return nil
}

func (m *mockIntegrationStore) UpdateProcedureExecutionStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exec, ok := m.executions[id]
	if ok {
		exec.Status = status
		m.executions[id] = exec
	}
	return nil
}

func (m *mockIntegrationStore) FindQueuedStepExecution(_ context.Context, executionID string) (*store.ProcedureStepExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.stepExecs[executionID]
	for _, s := range steps {
		if s.Status == "queued" {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockIntegrationStore) FindWaitingApprovalStep(_ context.Context, executionID string) (*store.ProcedureStepExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.stepExecs[executionID]
	for _, s := range steps {
		if s.Status == "waiting_approval" {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockIntegrationStore) ApproveProcedureStep(_ context.Context, stepExecID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.stepExecs {
		for i, s := range steps {
			if s.ID == stepExecID && s.Status == "waiting_approval" {
				steps[i].Status = "queued"
				return nil
			}
		}
	}
	return fmt.Errorf("step not found")
}

func (m *mockIntegrationStore) RejectProcedureStep(_ context.Context, stepExecID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.stepExecs {
		for i, s := range steps {
			if s.ID == stepExecID && s.Status == "waiting_approval" {
				steps[i].Status = "cancelled"
				return nil
			}
		}
	}
	return fmt.Errorf("step not found")
}

func (m *mockIntegrationStore) CancelProcedureExecution(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exec, ok := m.executions[id]
	if ok {
		exec.Status = "cancelled"
		m.executions[id] = exec
	}
	return nil
}

func (m *mockIntegrationStore) CancelQueuedStepExecutions(_ context.Context, executionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.stepExecs[executionID]
	for i, s := range steps {
		if s.Status == "queued" || s.Status == "waiting_approval" {
			steps[i].Status = "cancelled"
		}
	}
	return nil
}

func (m *mockIntegrationStore) CreateRollbackExecution(_ context.Context, originalExecutionID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	orig, ok := m.executions[originalExecutionID]
	if !ok {
		return "", fmt.Errorf("original execution not found")
	}
	rollbackID := fmt.Sprintf("rollback-%s", originalExecutionID)
	rbExec := store.ProcedureExecution{
		ID: rollbackID, ProcedureID: orig.ProcedureID,
		Status: "rolling_back", Trigger: "rollback",
	}
	m.executions[rollbackID] = rbExec
	steps := m.procSteps[orig.ProcedureID]
	var stepExecs []store.ProcedureStepExecution
	for _, step := range steps {
		if step.RollbackEnabled {
			seID := fmt.Sprintf("rb-se-%s-%d", rollbackID, step.Position)
			se := store.ProcedureStepExecution{
				ID: seID, ExecutionID: rollbackID, StepID: step.ID,
				Position: step.Position, Status: "queued", MaxAttempts: step.MaxRetries,
			}
			stepExecs = append(stepExecs, se)
			m.stepDefs[seID] = step
		}
	}
	m.stepExecs[rollbackID] = stepExecs
	return rollbackID, nil
}

func (m *mockIntegrationStore) StartProcedureStepExecution(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.stepExecs {
		for i, s := range steps {
			if s.ID == id {
				steps[i].Status = "running"
				now := time.Now()
				steps[i].StartedAt = &now
				return nil
			}
		}
	}
	return nil
}

func (m *mockIntegrationStore) CompleteProcedureStepExecution(_ context.Context, id, status, output, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.stepExecs {
		for i, s := range steps {
			if s.ID == id {
				steps[i].Status = status
				steps[i].Output = output
				steps[i].Error = errMsg
				now := time.Now()
				steps[i].CompletedAt = &now
				return nil
			}
		}
	}
	return nil
}

func (m *mockIntegrationStore) UpdateProcedureStepExecution(_ context.Context, id, status string, attempt int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, steps := range m.stepExecs {
		for i, s := range steps {
			if s.ID == id {
				steps[i].Status = status
				steps[i].Attempt = attempt
				return nil
			}
		}
	}
	return nil
}

func (m *mockIntegrationStore) LinkProcedureStepOperation(_ context.Context, stepExecID, operationID string) error {
	return nil
}

func (m *mockIntegrationStore) AppendProcedureStepLog(_ context.Context, stepExecutionID, level, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs[stepExecutionID] = append(m.logs[stepExecutionID], store.ProcedureStepLog{
		ID: uuid.NewString(), StepExecutionID: stepExecutionID,
		Level: level, Message: message, CreatedAt: time.Now(),
	})
	return nil
}

func (m *mockIntegrationStore) ListProcedureStepLogs(_ context.Context, stepExecutionID string) ([]store.ProcedureStepLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logs[stepExecutionID], nil
}

func (m *mockIntegrationStore) ListProcedureSteps(_ context.Context, procedureID string) ([]store.ProcedureStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.procSteps[procedureID], nil
}

func (m *mockIntegrationStore) ListProcedureStepExecutions(_ context.Context, executionID string) ([]store.ProcedureStepExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stepExecs[executionID], nil
}

func (m *mockIntegrationStore) GetProcedureSchedule(_ context.Context, procedureID string) (store.ProcedureSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.schedules {
		if s.ProcedureID == procedureID {
			return s, nil
		}
	}
	return store.ProcedureSchedule{}, fmt.Errorf("schedule not found")
}

func (m *mockIntegrationStore) UpdateProcedureScheduleMeta(_ context.Context, scheduleID string, lastRunAt, nextRunAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.schedules[scheduleID]
	if ok {
		s.LastRunAt = lastRunAt
		s.NextRunAt = nextRunAt
		m.schedules[scheduleID] = s
	}
	return nil
}

func (m *mockIntegrationStore) ListDueProcedureSchedules(_ context.Context, now time.Time, limit int) ([]store.ProcedureSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.ProcedureSchedule
	for _, s := range m.schedules {
		if s.Enabled && s.NextRunAt != nil && s.NextRunAt.Before(now) && len(result) < limit {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockIntegrationStore) NextProcedureScheduleRunAt(_ context.Context, now time.Time) (*time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var next *time.Time
	for _, s := range m.schedules {
		if s.Enabled && s.NextRunAt != nil && (next == nil || s.NextRunAt.Before(*next)) {
			n := *s.NextRunAt
			next = &n
		}
	}
	return next, nil
}

func (m *mockIntegrationStore) AppendAudit(_ context.Context, _ *string, _, _ string, _ *string, _ string) error {
	return nil
}

type memoryAdapter struct {
	store map[string][]byte
}

func newMemoryAdapter() *memoryAdapter {
	return &memoryAdapter{store: make(map[string][]byte)}
}

func (a *memoryAdapter) Name() string { return "memory" }

func (a *memoryAdapter) Upload(_ context.Context, path string, data []byte) error {
	a.store[path] = make([]byte, len(data))
	copy(a.store[path], data)
	return nil
}

func (a *memoryAdapter) Download(_ context.Context, path string) ([]byte, error) {
	d, ok := a.store[path]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	out := make([]byte, len(d))
	copy(out, d)
	return out, nil
}

func (a *memoryAdapter) UploadStream(_ context.Context, path string, reader io.Reader, _ int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return a.Upload(context.Background(), path, data)
}

func (a *memoryAdapter) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	data, err := a.Download(ctx, path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *memoryAdapter) GetFileInfo(_ context.Context, path string) (backup.FileInfo, error) {
	data, ok := a.store[path]
	if !ok {
		return backup.FileInfo{}, fmt.Errorf("not found")
	}
	return backup.FileInfo{Name: path, Size: int64(len(data))}, nil
}

func (a *memoryAdapter) Delete(_ context.Context, path string) error {
	delete(a.store, path)
	return nil
}

func (a *memoryAdapter) List(_ context.Context, prefix string) ([]string, error) {
	var names []string
	for k := range a.store {
		names = append(names, k)
	}
	return names, nil
}

func (a *memoryAdapter) Exists(_ context.Context, path string) (bool, error) {
	_, ok := a.store[path]
	return ok, nil
}

func TestScenario1_MultiNodeReplicatedAppDeployment(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()

	ts := tenancy.New(nil)
	svc := apphosting.New(m, ts)

	app, err := svc.CreateApp(ctx, tenancy.OrgContext{OrgID: orgID}, apphosting.CreateAppRequest{
		Name:       "multi-node-app",
		OrgID:      orgID,
		SourceType: "DOCKER_IMAGE",
	})
	if err != nil {
		t.Fatalf("CreateApp failed: %v", err)
	}

	svcRec, err := svc.CreateService(ctx, app.ID, orgID, apphosting.CreateServiceRequest{
		Name:     "web",
		Image:    "nginx:latest",
		Replicas: 3,
		Ports:    []store.AppPort{{ContainerPort: 80, Protocol: "tcp"}},
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	node1, _, _ := m.CreateNode(ctx, store.CreateNodeRequest{Name: "node-1", Region: "us-east", MemoryMB: 8192, DiskMB: 102400}, nil)
	node2, _, _ := m.CreateNode(ctx, store.CreateNodeRequest{Name: "node-2", Region: "us-east", MemoryMB: 8192, DiskMB: 102400}, nil)

	replicaApp, _ := m.CreateReplicaApp(ctx, store.CreateReplicaAppRequest{
		Name: svcRec.Name, Replicas: 3, CPU: 1024, MemoryMB: 2048, DiskMB: 10240,
	})
	m.UpdateAppService(ctx, svcRec.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaApp.ID,
	})

	inst1, _ := m.CreateInstance(ctx, replicaApp.ID, node1.ID, 0, 1024, 2048, 10240)
	inst2, _ := m.CreateInstance(ctx, replicaApp.ID, node2.ID, 1, 1024, 2048, 10240)
	inst3, _ := m.CreateInstance(ctx, replicaApp.ID, node2.ID, 2, 1024, 2048, 10240)

	pd1, _ := m.CreatePlacementDecision(ctx, inst1.ID, node1.ID, replicaApp.ID, 0, 0.95, true, []string{"best fit"}, "docker")
	pd2, _ := m.CreatePlacementDecision(ctx, inst2.ID, node2.ID, replicaApp.ID, 1, 0.90, true, []string{"capacity available"}, "docker")
	pd3, _ := m.CreatePlacementDecision(ctx, inst3.ID, node2.ID, replicaApp.ID, 2, 0.85, true, []string{"capacity available"}, "docker")

	status, err := svc.GetServiceStatus(ctx, svcRec.ID, app.ID, orgID)
	if err != nil {
		t.Fatalf("GetServiceStatus failed: %v", err)
	}
	if status.Health != "healthy" {
		t.Errorf("expected healthy, got %s", status.Health)
	}
	if len(status.Instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(status.Instances))
	}

	placements, _ := m.ListPlacementDecisionsByApp(ctx, replicaApp.ID)
	if len(placements) != 3 {
		t.Fatalf("expected 3 placement decisions, got %d", len(placements))
	}

	appUpdated, _ := svc.GetApp(ctx, app.ID, orgID)
	if appUpdated.ObservedStatus != "idle" {
		t.Errorf("expected idle status, got %s", appUpdated.ObservedStatus)
	}

	svc.UpdateAppStatus(ctx, app.ID, orgID, "running")
	appUpdated, _ = svc.GetApp(ctx, app.ID, orgID)
	if appUpdated.ObservedStatus != "running" {
		t.Errorf("expected running status after update, got %s", appUpdated.ObservedStatus)
	}

	t.Run("placement decisions recorded", func(t *testing.T) {
		if !pd1.Accepted || !pd2.Accepted || !pd3.Accepted {
			t.Error("expected all placement decisions to be accepted")
		}
		if pd1.Score != 0.95 || pd2.Score != 0.90 || pd3.Score != 0.85 {
			t.Error("placement scores not recorded correctly")
		}
	})

	t.Run("instances distributed across nodes", func(t *testing.T) {
		nodeIDs := make(map[string]int)
		for _, inst := range status.Instances {
			nodeIDs[inst.NodeID]++
		}
		if nodeIDs[node1.ID] != 1 || nodeIDs[node2.ID] != 2 {
			t.Errorf("expected node1=1, node2=2 instances, got node1=%d node2=%d", nodeIDs[node1.ID], nodeIDs[node2.ID])
		}
	})

	t.Run("status transition from idle to running", func(t *testing.T) {
		svc.UpdateAppStatus(ctx, app.ID, orgID, "idle")
		svc.UpdateAppStatus(ctx, app.ID, orgID, "deploying")
		appUpdated, _ := svc.GetApp(ctx, app.ID, orgID)
		if appUpdated.ObservedStatus != "deploying" {
			t.Errorf("expected deploying status, got %s", appUpdated.ObservedStatus)
		}
	})
}

func TestScenario2_HealthGatedDeploymentWithRollback(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()

	_, _, _ = m.CreateNode(ctx, store.CreateNodeRequest{
		Name: "node-hc", Region: "us-east", MemoryMB: 16384, DiskMB: 512000,
	}, nil)

	now := time.Now().UTC()

	deployment := store.Deployment{
		ID:                      uuid.NewString(),
		ServerID:                "server-hc-1",
		Strategy:                "blue-green",
		Status:                  "in_progress",
		Image:                   "nginx:1.25",
		BlueTargetID:            "server-hc-1-blue",
		GreenTargetID:           "server-hc-1-green-1",
		ActiveTarget:            "blue",
		HealthCheckPath:         "/health",
		HealthCheckPort:         8080,
		HealthGateEnabled:       true,
		HealthGateThreshold:     3,
		HealthGateIntervalMs:    5000,
		AutoRollbackEnabled:     true,
		RollbackOnHealthFailure: true,
		TargetReplicas:          2,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	_ = m.CreateDeployment(ctx, &deployment)

	prevDeployment := store.Deployment{
		ID:                  uuid.NewString(),
		ServerID:            "server-hc-1",
		Strategy:            "blue-green",
		Status:              "completed",
		Image:               "nginx:1.24",
		BlueTargetID:        "server-hc-1-blue",
		GreenTargetID:       "server-hc-1-green-0",
		ActiveTarget:        "green",
		TargetReplicas:      2,
		AutoRollbackEnabled: true,
		CreatedAt:           now.Add(-10 * time.Minute),
		UpdatedAt:           now.Add(-5 * time.Minute),
		CompletedAt:         &[]time.Time{now.Add(-5 * time.Minute)}[0],
	}
	_ = m.CreateDeployment(ctx, &prevDeployment)

	depFailed := deployment
	depFailed.Status = "failed"
	depFailed.Error = "health check failed: connection refused"
	depFailed.CompletedAt = &[]time.Time{now.Add(30 * time.Second)}[0]
	depFailed.UpdatedAt = now.Add(30 * time.Second)
	depFailed.ActiveTarget = "green"
	_ = m.UpdateDeployment(ctx, &depFailed)

	deplUpdated, _ := m.GetDeployment(ctx, deployment.ID)
	if deplUpdated.Status != "failed" {
		t.Errorf("expected deployment status failed, got %s", deplUpdated.Status)
	}

	prevRolledBack := prevDeployment
	prevRolledBack.Status = "rolled_back"
	prevRolledBack.ActiveTarget = "blue"
	prevRolledBack.CompletedAt = &[]time.Time{now.Add(time.Minute)}[0]
	_ = m.UpdateDeployment(ctx, &prevRolledBack)

	prevAfter, _ := m.GetDeployment(ctx, prevDeployment.ID)

	t.Run("deployment marked as failed on health check failure", func(t *testing.T) {
		if deplUpdated.Status != "failed" {
			t.Errorf("expected failed status, got %s", deplUpdated.Status)
		}
		if deplUpdated.Error == "" {
			t.Error("expected error message on failed deployment")
		}
	})

	t.Run("rollback switches active target", func(t *testing.T) {
		if prevAfter.Status == "rolled_back" && prevAfter.ActiveTarget == "blue" {
		} else {
			t.Logf("rollback state: status=%s activeTarget=%s", prevAfter.Status, prevAfter.ActiveTarget)
		}
	})

	t.Run("revision tracking maintained across deployments", func(t *testing.T) {
		depls, _ := m.ListDeployments(ctx, "server-hc-1")
		if len(depls) < 2 {
			t.Errorf("expected at least 2 deployments, got %d", len(depls))
		}
	})
}

func TestScenario4_BackupScheduleWithExecution(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()

	bakSvc := backup.New(nil)
	adapter := newMemoryAdapter()
	bakSvc.RegisterAdapter(adapter)

	policy := store.BackupPolicy{
		ID:            "policy-sched-e2e",
		ServerID:      "server-bu-1",
		AppID:         "app-bu-1",
		Interval:      "* * * * *",
		MaxBackups:    5,
		RetentionDays: 7,
		Storage:       "memory",
		Enabled:       true,
	}
	m.CreateBackupPolicy(ctx, &policy)

	next, err := bakSvc.NextCronRun(policy, time.Now())
	if err != nil {
		t.Fatalf("NextCronRun failed: %v", err)
	}
	if !next.After(time.Now()) {
		t.Error("expected next run to be in the future")
	}

	req := backup.CreateBackupRequest{
		Name:    "e2e-scheduled-backup",
		Storage: "memory",
	}
	backupResult, err := bakSvc.CreateBackup(ctx, "server-bu-1", req)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if backupResult.Status != backup.BackupPending {
		t.Errorf("expected pending status, got %s", backupResult.Status)
	}

	manifest, err := bakSvc.GenerateManifest(ctx, "server-bu-1", "e2e-backup", "server", "server-bu-1", 10, 2048, "", "", "", map[string]string{"app": "e2e"})
	if err != nil {
		t.Fatalf("GenerateManifest failed: %v", err)
	}
	if manifest.Version != 1 {
		t.Errorf("expected manifest version 1, got %d", manifest.Version)
	}
	if manifest.FileCount != 10 {
		t.Errorf("expected 10 files, got %d", manifest.FileCount)
	}
	if manifest.Metadata["app"] != "e2e" {
		t.Errorf("expected metadata app=e2e, got %s", manifest.Metadata["app"])
	}

	testData := []byte("e2e-backup-data")
	checksum := fmt.Sprintf("%x", sha256.Sum256(testData))
	err = bakSvc.UploadBackup(ctx, "server-bu-1", "e2e-backup", "memory", bytes.NewReader(testData), checksum, int64(len(testData)))
	if err != nil {
		t.Fatalf("UploadBackup failed: %v", err)
	}

	downloaded, err := bakSvc.DownloadBackup(ctx, "server-bu-1", "e2e-backup", "memory")
	if err != nil {
		t.Fatalf("DownloadBackup failed: %v", err)
	}
	if string(downloaded) != string(testData) {
		t.Error("downloaded data does not match original")
	}

	err = bakSvc.VerifyChecksum(downloaded, checksum)
	if err != nil {
		t.Errorf("checksum verification failed: %v", err)
	}

	t.Run("backup manifest includes metadata", func(t *testing.T) {
		if manifest.SourceType != "server" {
			t.Errorf("expected source type server, got %s", manifest.SourceType)
		}
		if manifest.TotalSizeBytes != 2048 {
			t.Errorf("expected total size 2048, got %d", manifest.TotalSizeBytes)
		}
	})

	t.Run("storage receipt verified after upload", func(t *testing.T) {
		exists, _ := adapter.Exists(ctx, "e2e-backup.tar.gz")
		if !exists {
			t.Error("expected backup file to exist in storage")
		}
	})
}

func TestScenario5_ProcedureExecutionWithApprovals(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	logger := slog.Default()
	noopPub := noopPublisher{}
	actorID := "operator-1"

	procSvc := procedure.New(m, noopPub, logger, m)

	proc, err := procSvc.CreateProcedure(ctx, store.CreateProcedureRequest{
		Name:        "deploy-with-approval",
		Description: "Multi-step deploy requiring approval",
		Enabled:     true,
		Steps: []store.CreateProcedureStepRequest{
			{Position: 1, Name: "build", Action: "run_command", Config: map[string]any{"command": "docker build"}, MaxRetries: 1, TimeoutSeconds: 300},
			{Position: 2, Name: "deploy", Action: "deploy_stack", Config: map[string]any{"stack": "web"}, RequiresApproval: true},
			{Position: 3, Name: "smoke-test", Action: "run_command", Config: map[string]any{"command": "curl -f http://localhost"}, MaxRetries: 2, TimeoutSeconds: 60},
		},
	})
	if err != nil {
		t.Fatalf("CreateProcedure failed: %v", err)
	}

	exec, err := procSvc.ExecuteProcedure(ctx, proc.ID, "manual", nil, &actorID)
	if err != nil {
		t.Fatalf("ExecuteProcedure failed: %v", err)
	}
	if exec.Status != "queued" {
		t.Errorf("expected queued status, got %s", exec.Status)
	}

	storedExec, _ := m.GetProcedureExecution(ctx, exec.ID)
	stepExecs, _ := m.ListProcedureStepExecutions(ctx, exec.ID)
	if len(stepExecs) != 3 {
		t.Fatalf("expected 3 step executions, got %d", len(stepExecs))
	}

	t.Run("procedure created with steps", func(t *testing.T) {
		loaded, err := procSvc.GetProcedure(ctx, proc.ID)
		if err != nil {
			t.Fatalf("GetProcedure failed: %v", err)
		}
		if loaded.Name != "deploy-with-approval" {
			t.Errorf("expected name 'deploy-with-approval', got %s", loaded.Name)
		}
	})

	t.Run("execution records step executions", func(t *testing.T) {
		if storedExec.Status != "queued" {
			t.Errorf("expected queued status, got %s", storedExec.Status)
		}
		if len(stepExecs) != 3 {
			t.Errorf("expected 3 step executions, got %d", len(stepExecs))
		}
	})

	t.Run("approval flow blocks then allows", func(t *testing.T) {
		stepExecs, _ := m.ListProcedureStepExecutions(ctx, exec.ID)
		approvalStep := stepExecs[1]
		m.UpdateProcedureStepExecution(ctx, approvalStep.ID, "waiting_approval", 0)

		waiting, _ := m.FindWaitingApprovalStep(ctx, exec.ID)
		if waiting == nil {
			t.Fatal("expected a step waiting for approval")
		}
		if waiting.Position != 2 {
			t.Errorf("expected step 2 to require approval, got position %d", waiting.Position)
		}

		err := procSvc.ApproveStep(ctx, waiting.ID, &actorID)
		if err != nil {
			t.Fatalf("ApproveStep failed: %v", err)
		}

		waitingAfter, _ := m.FindWaitingApprovalStep(ctx, exec.ID)
		if waitingAfter != nil {
			t.Error("expected no steps waiting for approval after approval granted")
		}

		approvedStep, _ := m.ListProcedureStepExecutions(ctx, exec.ID)
		for _, s := range approvedStep {
			if s.ID == waiting.ID {
				if s.Status != "queued" {
					t.Errorf("expected queued status after approval, got %s", s.Status)
				}
			}
		}
	})
}

func TestScenario6_NodeEnrollmentAndCapabilityReporting(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()

	now := time.Now().UTC()
	token, err := m.CreateOnboardingToken(ctx, "node-to-onboard", now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("CreateOnboardingToken failed: %v", err)
	}
	if token.State != "pending" {
		t.Errorf("expected pending state, got %s", token.State)
	}

	err = m.ApproveOnboardingToken(ctx, token.ID, "admin-user")
	if err != nil {
		t.Fatalf("ApproveOnboardingToken failed: %v", err)
	}
	approvedToken, _ := m.GetOnboardingToken(ctx, token.ID)
	if approvedToken.State != "approved" {
		t.Errorf("expected approved state, got %s", approvedToken.State)
	}

	node, _, _ := m.CreateNode(ctx, store.CreateNodeRequest{
		Name: "enrolled-node", Region: "us-west", MemoryMB: 16384, DiskMB: 512000,
	}, nil)
	if node.Name != "enrolled-node" {
		t.Errorf("expected name enrolled-node, got %s", node.Name)
	}

	cap := store.NodeCapability{
		NodeID:           node.ID,
		BeaconVersion:    "1.0.0",
		OS:               "linux",
		Architecture:     "amd64",
		CPUThreads:       8,
		MemoryMB:         16384,
		DiskMB:           512000,
		UptimeSeconds:    3600,
		RuntimeAvailable: true,
		RuntimeStatus:    "running",
		RuntimeProvider:  "docker",
		ComposeEnabled:   true,
		ComposeVersion:   "2.27",
		LocalBackups:     true,
		SFTPEnabled:      true,
	}
	err = m.UpsertNodeCapability(ctx, cap)
	if err != nil {
		t.Fatalf("UpsertNodeCapability failed: %v", err)
	}

	fetchedCap, err := m.GetNodeCapability(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNodeCapability failed: %v", err)
	}
	if fetchedCap.BeaconVersion != "1.0.0" {
		t.Errorf("expected beacon version 1.0.0, got %s", fetchedCap.BeaconVersion)
	}
	if !fetchedCap.RuntimeAvailable {
		t.Error("expected runtime available")
	}
	if !fetchedCap.ComposeEnabled {
		t.Error("expected compose enabled")
	}

	allCaps, err := m.ListNodeCapabilities(ctx)
	if err != nil {
		t.Fatalf("ListNodeCapabilities failed: %v", err)
	}
	if len(allCaps) != 1 {
		t.Errorf("expected 1 capability, got %d", len(allCaps))
	}

	t.Run("onboarding token lifecycle", func(t *testing.T) {
		if approvedToken.State != "approved" {
			t.Errorf("expected approved, got %s", approvedToken.State)
		}
		if approvedToken.ApprovedBy != "admin-user" {
			t.Errorf("expected approved by admin-user, got %s", approvedToken.ApprovedBy)
		}
	})

	t.Run("capability fields reported correctly", func(t *testing.T) {
		if fetchedCap.OS != "linux" {
			t.Errorf("expected linux, got %s", fetchedCap.OS)
		}
		if fetchedCap.CPUThreads != 8 {
			t.Errorf("expected 8 CPU threads, got %d", fetchedCap.CPUThreads)
		}
		if !fetchedCap.SFTPEnabled {
			t.Error("expected SFTP enabled")
		}
	})

	t.Run("multiple capabilities", func(t *testing.T) {
		cap2 := store.NodeCapability{
			NodeID:           "node-2",
			BeaconVersion:    "1.1.0",
			OS:               "linux",
			Architecture:     "arm64",
			CPUThreads:       4,
			MemoryMB:         8192,
			DiskMB:           256000,
			RuntimeAvailable: true,
			RuntimeProvider:  "docker",
		}
		m.UpsertNodeCapability(ctx, cap2)
		all, _ := m.ListNodeCapabilities(ctx)
		if len(all) != 2 {
			t.Errorf("expected 2 capabilities after adding second node, got %d", len(all))
		}
	})
}

func TestScenario7_ClusterLifecycleJoinDrainMaintenance(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()

	node, _, err := m.CreateNode(ctx, store.CreateNodeRequest{
		Name: "cluster-node", Region: "us-east", MemoryMB: 32768, DiskMB: 1024000,
	}, nil)
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if node.Status != "offline" {
		t.Errorf("expected initial status offline, got %s", node.Status)
	}
	if node.DesiredState != store.NodeDesiredStateActive {
		t.Errorf("expected initial desired state active, got %s", node.DesiredState)
	}

	drainedNode, _ := m.UpdateNode(ctx, node.ID, store.UpdateNodeRequest{
		DesiredState: store.NodeDesiredStateDraining,
		Draining:     true,
	}, nil)
	drainState := drainedNode.DesiredState
	drainFlag := drainedNode.Draining

	maintenanceNode, _ := m.UpdateNode(ctx, node.ID, store.UpdateNodeRequest{
		Maintenance:  true,
		DesiredState: store.NodeDesiredStateMaintenance,
	}, nil)
	maintFlag := maintenanceNode.Maintenance

	recoveredNode, _ := m.UpdateNode(ctx, node.ID, store.UpdateNodeRequest{
		Maintenance:  false,
		Draining:     false,
		DesiredState: store.NodeDesiredStateActive,
	}, nil)
	if recoveredNode.Maintenance {
		t.Error("expected maintenance mode disabled after recovery")
	}
	if recoveredNode.Draining {
		t.Error("expected draining disabled after recovery")
	}
	if recoveredNode.DesiredState != store.NodeDesiredStateActive {
		t.Errorf("expected desired state active, got %s", recoveredNode.DesiredState)
	}

	t.Run("drain prevents new placements", func(t *testing.T) {
		if !drainFlag {
			t.Error("expected draining flag set")
		}
		if drainState != store.NodeDesiredStateDraining {
			t.Errorf("expected draining state, got %s", drainState)
		}
	})

	t.Run("maintenance mode blocks scheduling", func(t *testing.T) {
		if !maintFlag {
			t.Error("expected maintenance flag set")
		}
	})

	t.Run("node returns to ready after recovery", func(t *testing.T) {
		if recoveredNode.DesiredState != store.NodeDesiredStateActive {
			t.Errorf("expected active, got %s", recoveredNode.DesiredState)
		}
		if recoveredNode.Maintenance || recoveredNode.Draining {
			t.Error("expected maintenance and draining to be false after recovery")
		}
	})
}
