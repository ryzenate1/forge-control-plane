package apphosting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

var errNotFound = errors.New("not found")

type mockInstance struct {
	store.Instance
}

type mockEndpoint struct {
	store.ServiceEndpoint
}

type mockStore struct {
	apps        map[string]*store.Application
	services    map[string][]store.AppService
	instances   map[string][]store.Instance
	endpoints   map[string][]store.ServiceEndpoint
	replicaApps map[string]store.ReplicaApplication
}

func newMockStore() *mockStore {
	return &mockStore{
		apps:        make(map[string]*store.Application),
		services:    make(map[string][]store.AppService),
		instances:   make(map[string][]store.Instance),
		endpoints:   make(map[string][]store.ServiceEndpoint),
		replicaApps: make(map[string]store.ReplicaApplication),
	}
}

func (m *mockStore) CreateApplication(ctx context.Context, input store.CreateApplicationInput) (*store.Application, error) {
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

func (m *mockStore) GetApplication(ctx context.Context, id string) (*store.Application, error) {
	app, ok := m.apps[id]
	if !ok {
		return nil, errNotFound
	}
	return app, nil
}

func (m *mockStore) ListApplications(ctx context.Context, orgID string) ([]store.Application, error) {
	var result []store.Application
	for _, app := range m.apps {
		if app.OrgID == orgID {
			result = append(result, *app)
		}
	}
	return result, nil
}

func (m *mockStore) UpdateApplication(ctx context.Context, id string, input store.UpdateApplicationInput) error {
	app, ok := m.apps[id]
	if !ok {
		return errNotFound
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

func (m *mockStore) DeleteApplication(ctx context.Context, id string) error {
	if _, ok := m.apps[id]; !ok {
		return errNotFound
	}
	delete(m.apps, id)
	delete(m.services, id)
	delete(m.instances, id)
	return nil
}

func (m *mockStore) UpdateApplicationStatus(ctx context.Context, id string, status string) error {
	app, ok := m.apps[id]
	if !ok {
		return errNotFound
	}
	app.ObservedStatus = status
	return nil
}

func (m *mockStore) SetApplicationDeployment(ctx context.Context, appID string, deploymentID *string) error {
	app, ok := m.apps[appID]
	if !ok {
		return errNotFound
	}
	app.CurrentDeploymentID = deploymentID
	return nil
}

func (m *mockStore) AppBelongsToOrg(ctx context.Context, appID, orgID string) (bool, error) {
	app, ok := m.apps[appID]
	if !ok {
		return false, nil
	}
	return app.OrgID == orgID, nil
}

func (m *mockStore) AppServiceBelongsToApp(ctx context.Context, serviceID, appID string) (bool, error) {
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

func (m *mockStore) CreateAppService(ctx context.Context, input store.CreateAppServiceInput) (*store.AppService, error) {
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

func (m *mockStore) GetAppService(ctx context.Context, id string) (*store.AppService, error) {
	for _, services := range m.services {
		for _, s := range services {
			if s.ID == id {
				return &s, nil
			}
		}
	}
	return nil, errNotFound
}

func (m *mockStore) ListAppServices(ctx context.Context, appID string) ([]store.AppService, error) {
	return m.services[appID], nil
}

func (m *mockStore) DeleteAppService(ctx context.Context, id string) error {
	for appID, services := range m.services {
		for i, s := range services {
			if s.ID == id {
				m.services[appID] = append(services[:i], services[i+1:]...)
				return nil
			}
		}
	}
	return errNotFound
}

func (m *mockStore) CreateDeployment(ctx context.Context, d *store.Deployment) error {
	return nil
}

func (m *mockStore) UpdateAppService(ctx context.Context, id string, input store.UpdateAppServiceInput) (*store.AppService, error) {
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
	return nil, errNotFound
}

func (m *mockStore) ListInstancesByApp(ctx context.Context, appID string) ([]store.Instance, error) {
	insts, ok := m.instances[appID]
	if !ok {
		return []store.Instance{}, nil
	}
	return insts, nil
}

func (m *mockStore) UpdateReplicaAppReplicas(ctx context.Context, appID string, replicas int) (store.ReplicaApplication, error) {
	app, ok := m.replicaApps[appID]
	if !ok {
		return store.ReplicaApplication{}, errNotFound
	}
	app.Replicas = replicas
	m.replicaApps[appID] = app
	return app, nil
}

func (m *mockStore) ListServiceEndpoints(ctx context.Context, serviceID string) ([]store.ServiceEndpoint, error) {
	eps, ok := m.endpoints[serviceID]
	if !ok {
		return []store.ServiceEndpoint{}, nil
	}
	return eps, nil
}

func (m *mockStore) CreateServiceEndpoint(ctx context.Context, serviceID, host string, port int, protocol string, nodeID, instanceID *string) (*store.ServiceEndpoint, error) {
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

func (m *mockStore) DeleteServiceEndpoint(ctx context.Context, id string) error {
	for serviceID, eps := range m.endpoints {
		for i, ep := range eps {
			if ep.ID == id {
				m.endpoints[serviceID] = append(eps[:i], eps[i+1:]...)
				return nil
			}
		}
	}
	return errNotFound
}

// helpers

func createTestApp(m *mockStore, ctx context.Context, orgID, name string) *store.Application {
	app, _ := m.CreateApplication(ctx, store.CreateApplicationInput{
		Name:       name,
		OrgID:      orgID,
		SourceType: "DOCKER_IMAGE",
	})
	return app
}

func createTestService(m *mockStore, ctx context.Context, appID, name, image string, replicas int) *store.AppService {
	svc, _ := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    appID,
		Name:     name,
		Image:    image,
		Replicas: replicas,
		Ports:    []store.AppPort{{ContainerPort: 80, Protocol: "tcp"}},
	})
	return svc
}

func createTestInstances(m *mockStore, appID, nodeID string, count int, status string) {
	var insts []store.Instance
	for i := 0; i < count; i++ {
		insts = append(insts, store.Instance{
			ID:     uuid.NewString(),
			AppID:  appID,
			Idx:    i,
			NodeID: nodeID,
			Status: status,
			CPU:    1024,
			MemoryMB: 2048,
			DiskMB:   10240,
		})
	}
	m.instances[appID] = insts
}

// ---- Tests ----

func TestCreateApp(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()

	app, err := m.CreateApplication(ctx, store.CreateApplicationInput{
		Name:         "test-app",
		OrgID:        orgID,
		SourceType:   "DOCKER_IMAGE",
		SourceConfig: json.RawMessage(`{"image":"nginx:latest"}`),
	})
	if err != nil {
		t.Fatalf("CreateApplication failed: %v", err)
	}
	if app.Name != "test-app" {
		t.Errorf("expected name 'test-app', got %q", app.Name)
	}
	if app.DesiredState != "running" {
		t.Errorf("expected desired_state 'running', got %q", app.DesiredState)
	}
	if app.ObservedStatus != "idle" {
		t.Errorf("expected observed_status 'idle', got %q", app.ObservedStatus)
	}
}

func TestValidateSourceType(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		valid      bool
	}{
		{"git uppercase", "GIT", true},
		{"docker image", "DOCKER_IMAGE", true},
		{"compose", "COMPOSE", true},
		{"invalid", "INVALID", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := validSourceTypes[tt.sourceType]
			if ok != tt.valid {
				t.Errorf("sourceType %q: expected valid=%v, got %v", tt.sourceType, tt.valid, ok)
			}
		})
	}
}

func TestAppBelongsToOrg(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	otherOrgID := uuid.NewString()

	app, _ := m.CreateApplication(ctx, store.CreateApplicationInput{
		Name: "test-app", OrgID: orgID, SourceType: "DOCKER_IMAGE",
	})

	belongs, _ := m.AppBelongsToOrg(ctx, app.ID, orgID)
	if !belongs {
		t.Error("expected app to belong to org")
	}
	belongs, _ = m.AppBelongsToOrg(ctx, app.ID, otherOrgID)
	if belongs {
		t.Error("expected app NOT to belong to other org")
	}
}

func TestListAppsByOrg(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()

	for _, name := range []string{"app-alpha", "app-beta", "app-gamma"} {
		_, err := m.CreateApplication(ctx, store.CreateApplicationInput{
			Name: name, OrgID: orgID, SourceType: "DOCKER_IMAGE",
		})
		if err != nil {
			t.Fatalf("CreateApplication failed: %v", err)
		}
	}

	apps, _ := m.ListApplications(ctx, orgID)
	if len(apps) != 3 {
		t.Errorf("expected 3 apps, got %d", len(apps))
	}
}

func TestCreateAppService(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "test-app")

	svc, err := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    app.ID,
		Name:     "web",
		Image:    "nginx:latest",
		Replicas: 2,
		Ports:    []store.AppPort{{ContainerPort: 80, Protocol: "tcp"}},
	})
	if err != nil {
		t.Fatalf("CreateAppService failed: %v", err)
	}
	if svc.Name != "web" {
		t.Errorf("expected 'web', got %q", svc.Name)
	}
	if svc.Replicas != 2 {
		t.Errorf("expected 2 replicas, got %d", svc.Replicas)
	}
	if len(svc.Ports) != 1 {
		t.Errorf("expected 1 port, got %d", len(svc.Ports))
	}
}

func TestDeleteAppCascadesToServices(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "test-app")
	createTestService(m, ctx, app.ID, "worker", "worker:latest", 1)

	if err := m.DeleteApplication(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApplication failed: %v", err)
	}
	services, _ := m.ListAppServices(ctx, app.ID)
	if len(services) != 0 {
		t.Error("expected no services after app deletion")
	}
}

// ---- Uncloud-inspired Tests ----

func TestCreateServiceWithUncloudDefaults(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")

	svc, err := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    app.ID,
		Name:     "frontend",
		Image:    "nginx:latest",
		Replicas: 1,
	})
	if err != nil {
		t.Fatalf("CreateAppService: %v", err)
	}

	if svc.Mode != "replicated" {
		t.Errorf("expected default mode 'replicated', got %q", svc.Mode)
	}
	if svc.UpdateConfig.Strategy != "" {
		t.Errorf("expected empty update config strategy for basic create, got %q", svc.UpdateConfig.Strategy)
	}
	if svc.Resources.CPU != 0 {
		t.Errorf("expected zero-value resources for basic create, got %d", svc.Resources.CPU)
	}
	if len(svc.Volumes) != 0 {
		t.Errorf("expected no volumes, got %d", len(svc.Volumes))
	}
	if svc.DesiredState != "running" {
		t.Errorf("expected desired 'running', got %q", svc.DesiredState)
	}
	if svc.ObservedStatus != "idle" {
		t.Errorf("expected observed 'idle', got %q", svc.ObservedStatus)
	}
}

func TestCreateServiceWithModeAndResources(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")

	svc, err := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    app.ID,
		Name:     "worker",
		Image:    "worker:latest",
		Replicas: 3,
		Mode:     "global",
		Resources: store.ResourceSpec{
			CPU:      2048,
			MemoryMB: 4096,
			DiskMB:   20480,
		},
		Volumes: []store.VolumeRef{
			{Name: "data", Target: "/data", Type: "volume"},
		},
		Secrets: []store.SecretRef{
			{Name: "db-password", Target: "DB_PASSWORD"},
		},
		UpdateConfig: store.UpdateConfig{
			Strategy:      "rolling",
			RollingOrder:  "stop-first",
			MonitorPeriod: 60,
		},
		HealthCheck: store.HealthCheckConfig{
			Path:     "/health",
			Port:     8080,
			Interval: 10,
			Timeout:  5,
			Retries:  3,
		},
	})
	if err != nil {
		t.Fatalf("CreateAppService: %v", err)
	}

	if svc.Mode != "global" {
		t.Errorf("expected 'global', got %q", svc.Mode)
	}
	if svc.Resources.CPU != 2048 {
		t.Errorf("expected CPU 2048, got %d", svc.Resources.CPU)
	}
	if svc.Resources.MemoryMB != 4096 {
		t.Errorf("expected MemoryMB 4096, got %d", svc.Resources.MemoryMB)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0].Name != "data" {
		t.Errorf("expected volume 'data', got %+v", svc.Volumes)
	}
	if len(svc.Secrets) != 1 || svc.Secrets[0].Name != "db-password" {
		t.Errorf("expected secret 'db-password', got %+v", svc.Secrets)
	}
	if svc.UpdateConfig.Strategy != "rolling" {
		t.Errorf("expected strategy 'rolling', got %q", svc.UpdateConfig.Strategy)
	}
	if svc.UpdateConfig.RollingOrder != "stop-first" {
		t.Errorf("expected rolling_order 'stop-first', got %q", svc.UpdateConfig.RollingOrder)
	}
	if svc.HealthCheck.Path != "/health" {
		t.Errorf("expected health path '/health', got %q", svc.HealthCheck.Path)
	}
}

func TestUpdateServiceImage(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:1.25", 2)

	updated, err := m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		Image: strPtr("nginx:1.26"),
	})
	if err != nil {
		t.Fatalf("UpdateAppService: %v", err)
	}
	if updated.Image != "nginx:1.26" {
		t.Errorf("expected image 'nginx:1.26', got %q", updated.Image)
	}
}

func TestUpdateServiceReplicas(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	updated, err := m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		Replicas: intPtr(5),
	})
	if err != nil {
		t.Fatalf("UpdateAppService: %v", err)
	}
	if updated.Replicas != 5 {
		t.Errorf("expected 5 replicas, got %d", updated.Replicas)
	}
}

func TestUpdateServiceMode(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	updated, err := m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		Mode: strPtr("global"),
	})
	if err != nil {
		t.Fatalf("UpdateAppService: %v", err)
	}
	if updated.Mode != "global" {
		t.Errorf("expected mode 'global', got %q", updated.Mode)
	}
}

// ---- Scale Tests ----

func TestScaleUp(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	m.replicaApps[replicaAppID] = store.ReplicaApplication{
		ID:       replicaAppID,
		Name:     "web",
		Replicas: 1,
	}

	svcSvc := &Service{store: m}

	updated, err := svcSvc.ScaleService(ctx, svc.ID, app.ID, orgID, 3)
	if err != nil {
		t.Fatalf("ScaleService: %v", err)
	}
	if updated.Replicas != 3 {
		t.Errorf("expected 3 replicas after scale up, got %d", updated.Replicas)
	}
	ra, ok := m.replicaApps[replicaAppID]
	if !ok || ra.Replicas != 3 {
		t.Errorf("expected replica_app replicas=3, got %d", ra.Replicas)
	}
}

func TestScaleDown(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 5)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	m.replicaApps[replicaAppID] = store.ReplicaApplication{
		ID:       replicaAppID,
		Name:     "web",
		Replicas: 5,
	}

	svcSvc := &Service{store: m}

	updated, err := svcSvc.ScaleService(ctx, svc.ID, app.ID, orgID, 2)
	if err != nil {
		t.Fatalf("ScaleService: %v", err)
	}
	if updated.Replicas != 2 {
		t.Errorf("expected 2 replicas after scale down, got %d", updated.Replicas)
	}
	ra, ok := m.replicaApps[replicaAppID]
	if !ok || ra.Replicas != 2 {
		t.Errorf("expected replica_app replicas=2, got %d", ra.Replicas)
	}
}

func TestScaleToZero(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 3)

	svcSvc := &Service{store: m}

	_, err := svcSvc.ScaleService(ctx, svc.ID, app.ID, orgID, 0)
	if err != nil {
		t.Fatalf("ScaleService to zero: %v", err)
	}
}

func TestScaleToNegativeFails(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	svcSvc := &Service{store: m}

	_, err := svcSvc.ScaleService(ctx, svc.ID, app.ID, orgID, -1)
	if err == nil {
		t.Fatal("expected error for negative replicas")
	}
}

// ---- Service Status Tests ----

func TestGetServiceStatusOneReplica(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	createTestInstances(m, replicaAppID, "node-1", 1, "running")

	svcSvc := &Service{store: m}

	status, err := svcSvc.GetServiceStatus(ctx, svc.ID, app.ID, orgID)
	if err != nil {
		t.Fatalf("GetServiceStatus: %v", err)
	}
	if status.Health != "healthy" {
		t.Errorf("expected 'healthy', got %q", status.Health)
	}
	if len(status.Instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(status.Instances))
	}
}

func TestGetServiceStatusMultipleReplicas(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 3)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	createTestInstances(m, replicaAppID, "node-1", 3, "running")

	svcSvc := &Service{store: m}

	status, _ := svcSvc.GetServiceStatus(ctx, svc.ID, app.ID, orgID)
	if len(status.Instances) != 3 {
		t.Errorf("expected 3 instances, got %d", len(status.Instances))
	}
	for _, inst := range status.Instances {
		if inst.Status != "running" {
			t.Errorf("expected instance %s to be running, got %s", inst.InstanceID, inst.Status)
		}
	}
}

func TestGetServiceStatusFailedInstance(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	insts := []store.Instance{
		{ID: uuid.NewString(), AppID: replicaAppID, Idx: 0, NodeID: "node-1", Status: "running", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		{ID: uuid.NewString(), AppID: replicaAppID, Idx: 1, NodeID: "node-1", Status: "failed", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
	}
	m.instances[replicaAppID] = insts

	svcSvc := &Service{store: m}

	status, _ := svcSvc.GetServiceStatus(ctx, svc.ID, app.ID, orgID)
	if status.Health != "degraded" {
		t.Errorf("expected 'degraded', got %q", status.Health)
	}
}

func TestGetServiceStatusReplacement(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	replicaAppID := uuid.NewString()
	m.UpdateAppService(ctx, svc.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID,
	})
	insts := []store.Instance{
		{ID: uuid.NewString(), AppID: replicaAppID, Idx: 0, NodeID: "node-1", Status: "running", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		{ID: uuid.NewString(), AppID: replicaAppID, Idx: 1, NodeID: "node-1", Status: "removing", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		{ID: uuid.NewString(), AppID: replicaAppID, Idx: 2, NodeID: "node-2", Status: "running", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
	}
	m.instances[replicaAppID] = insts

	svcSvc := &Service{store: m}

	status, _ := svcSvc.GetServiceStatus(ctx, svc.ID, app.ID, orgID)
	if len(status.Instances) != 3 {
		t.Errorf("expected 3 instances (2 running + 1 removing), got %d", len(status.Instances))
	}
	if status.Health != "healthy" {
		t.Errorf("expected 'healthy' (2 running out of 3 total), got %q", status.Health)
	}
}

func TestGetServiceStatusNoInstances(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 3)

	svcSvc := &Service{store: m}

	status, _ := svcSvc.GetServiceStatus(ctx, svc.ID, app.ID, orgID)
	if status.Health != "unknown" {
		t.Errorf("expected 'unknown' health when no instances, got %q", status.Health)
	}
}

// ---- Service Endpoint Tests ----

func TestServiceEndpoints(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	nodeID := uuid.NewString()
	ep, err := m.CreateServiceEndpoint(ctx, svc.ID, "10.0.0.1", 30080, "tcp", &nodeID, nil)
	if err != nil {
		t.Fatalf("CreateServiceEndpoint: %v", err)
	}
	if ep.Host != "10.0.0.1" {
		t.Errorf("expected host '10.0.0.1', got %q", ep.Host)
	}
	if ep.Port != 30080 {
		t.Errorf("expected port 30080, got %d", ep.Port)
	}

	svcSvc := &Service{store: m}

	endpoints, err := svcSvc.GetServiceEndpoints(ctx, svc.ID, app.ID, orgID)
	if err != nil {
		t.Fatalf("GetServiceEndpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(endpoints))
	}
}

func TestServiceEndpointsCrossOrgFails(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	otherOrgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	svcSvc := &Service{store: m}

	_, err := svcSvc.GetServiceEndpoints(ctx, svc.ID, app.ID, otherOrgID)
	if err == nil {
		t.Fatal("expected error for cross-org access")
	}
}

// ---- Service Overview Tests ----

func TestGetServiceOverview(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")

	svc1 := createTestService(m, ctx, app.ID, "web", "nginx:latest", 3)
	svc2 := createTestService(m, ctx, app.ID, "worker", "worker:latest", 2)

	replicaAppID1 := uuid.NewString()
	replicaAppID2 := uuid.NewString()

	m.UpdateAppService(ctx, svc1.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID1,
	})
	m.UpdateAppService(ctx, svc2.ID, store.UpdateAppServiceInput{
		ReplicaAppID: &replicaAppID2,
	})

	createTestInstances(m, replicaAppID1, "node-1", 3, "running")
	createTestInstances(m, replicaAppID2, "node-1", 1, "running")

	m.CreateServiceEndpoint(ctx, svc1.ID, "10.0.0.1", 30080, "tcp", nil, nil)

	svcSvc := &Service{store: m}

	overviews, err := svcSvc.GetServiceOverview(ctx, app.ID, orgID)
	if err != nil {
		t.Fatalf("GetServiceOverview: %v", err)
	}
	if len(overviews) != 2 {
		t.Fatalf("expected 2 services, got %d", len(overviews))
	}

	var webOverview *ServiceOverview
	for i := range overviews {
		if overviews[i].Name == "web" {
			webOverview = &overviews[i]
			break
		}
	}
	if webOverview == nil {
		t.Fatal("expected 'web' in overview")
	}
	if webOverview.Running != 3 {
		t.Errorf("expected 3 running for web, got %d", webOverview.Running)
	}
	if webOverview.Desired != 3 {
		t.Errorf("expected desired 3, got %d", webOverview.Desired)
	}
	if webOverview.Health != "healthy" {
		t.Errorf("expected 'healthy', got %q", webOverview.Health)
	}
	if !webOverview.HasEndpoint {
		t.Error("expected web to have endpoint")
	}
}

// ---- Plan/Apply Tests ----

func TestPlanServiceUpdateImage(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:1.25", 2)

	svcSvc := &Service{store: m}

	plan, err := svcSvc.PlanServiceUpdate(ctx, svc.ID, app.ID, orgID, UpdateServiceRequest{
		Image: strPtr("nginx:1.26"),
	})
	if err != nil {
		t.Fatalf("PlanServiceUpdate: %v", err)
	}
	if plan.DesiredImage != "nginx:1.26" {
		t.Errorf("expected desired image 'nginx:1.26', got %q", plan.DesiredImage)
	}
	if len(plan.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(plan.Changes))
	}
	if plan.Changes[0].Field != "image" {
		t.Errorf("expected field 'image', got %q", plan.Changes[0].Field)
	}
}

func TestPlanServiceUpdateReplicas(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	svcSvc := &Service{store: m}

	plan, err := svcSvc.PlanServiceUpdate(ctx, svc.ID, app.ID, orgID, UpdateServiceRequest{
		Replicas: intPtr(5),
	})
	if err != nil {
		t.Fatalf("PlanServiceUpdate: %v", err)
	}
	if plan.DesiredReplicas != 5 {
		t.Errorf("expected desired 5 replicas, got %d", plan.DesiredReplicas)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Field != "replicas" {
		t.Errorf("expected replicas change, got %+v", plan.Changes)
	}
}

func TestPlanServiceUpdateNoChanges(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 2)

	svcSvc := &Service{store: m}

	plan, err := svcSvc.PlanServiceUpdate(ctx, svc.ID, app.ID, orgID, UpdateServiceRequest{})
	if err != nil {
		t.Fatalf("PlanServiceUpdate: %v", err)
	}
	if len(plan.Changes) != 0 {
		t.Errorf("expected no changes, got %d", len(plan.Changes))
	}
}

func TestApplyServiceUpdate(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:1.25", 2)

	svcSvc := &Service{store: m}

	plan, _ := svcSvc.PlanServiceUpdate(ctx, svc.ID, app.ID, orgID, UpdateServiceRequest{
		Image:    strPtr("nginx:1.26"),
		Replicas: intPtr(4),
	})

	updated, err := svcSvc.ApplyServiceUpdate(ctx, svc.ID, app.ID, orgID, plan)
	if err != nil {
		t.Fatalf("ApplyServiceUpdate: %v", err)
	}
	if updated.Image != "nginx:1.26" {
		t.Errorf("expected image 'nginx:1.26', got %q", updated.Image)
	}
	if updated.Replicas != 4 {
		t.Errorf("expected 4 replicas, got %d", updated.Replicas)
	}
}

func TestApplyServiceUpdateNoPlanFails(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	svcSvc := &Service{store: m}

	_, err := svcSvc.ApplyServiceUpdate(ctx, svc.ID, app.ID, orgID, nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

// ---- Persistent Volume Restriction ----

func TestServiceWithPersistentVolumeRef(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")

	svc, err := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    app.ID,
		Name:     "db",
		Image:    "postgres:16",
		Replicas: 1,
		Volumes: []store.VolumeRef{
			{Name: "pgdata", Target: "/var/lib/postgresql/data", Type: "volume", ReadOnly: false},
		},
	})
	if err != nil {
		t.Fatalf("CreateAppService with volume: %v", err)
	}
	if len(svc.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(svc.Volumes))
	}
	if svc.Volumes[0].Name != "pgdata" {
		t.Errorf("expected volume 'pgdata', got %q", svc.Volumes[0].Name)
	}
	if svc.Volumes[0].Target != "/var/lib/postgresql/data" {
		t.Errorf("expected target '/var/lib/postgresql/data', got %q", svc.Volumes[0].Target)
	}
}

// ---- Project Isolation ----

func TestServiceScopeToProject(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	projectID := uuid.NewString()

	app, err := m.CreateApplication(ctx, store.CreateApplicationInput{
		Name:       "project-app",
		OrgID:      orgID,
		ProjectID:  &projectID,
		SourceType: "DOCKER_IMAGE",
	})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if app.ProjectID == nil || *app.ProjectID != projectID {
		t.Errorf("expected project ID %q, got %v", projectID, app.ProjectID)
	}

	svc, err := m.CreateAppService(ctx, store.CreateAppServiceInput{
		AppID:    app.ID,
		Name:     "scoped-service",
		Image:    "nginx:latest",
		Replicas: 1,
	})
	if err != nil {
		t.Fatalf("CreateAppService: %v", err)
	}
	if svc.AppID != app.ID {
		t.Errorf("expected service app_id %q, got %q", app.ID, svc.AppID)
	}
}

func TestCrossOrgAccessDenied(t *testing.T) {
	m := newMockStore()
	ctx := context.Background()
	orgID := uuid.NewString()
	otherOrgID := uuid.NewString()
	app := createTestApp(m, ctx, orgID, "myapp")
	svc := createTestService(m, ctx, app.ID, "web", "nginx:latest", 1)

	svcSvc := &Service{store: m}

	_, err := svcSvc.UpdateService(ctx, svc.ID, app.ID, otherOrgID, UpdateServiceRequest{
		Image: strPtr("nginx:latest"),
	})
	if err == nil {
		t.Fatal("expected error for cross-org update")
	}
}

// ---- ComputeServiceHealth ----

func TestComputeServiceHealth(t *testing.T) {
	tests := []struct {
		name      string
		instances []ServiceInstanceStatus
		expected  string
	}{
		{
			name:      "empty",
			instances: []ServiceInstanceStatus{},
			expected:  "unknown",
		},
		{
			name: "all running",
			instances: []ServiceInstanceStatus{
				{Status: "running"},
				{Status: "running"},
			},
			expected: "healthy",
		},
		{
			name: "some failed",
			instances: []ServiceInstanceStatus{
				{Status: "running"},
				{Status: "failed"},
			},
			expected: "degraded",
		},
		{
			name: "partial",
			instances: []ServiceInstanceStatus{
				{Status: "running"},
				{Status: "pending"},
			},
			expected: "partial",
		},
		{
			name: "all failed",
			instances: []ServiceInstanceStatus{
				{Status: "failed"},
				{Status: "failed"},
			},
			expected: "unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeServiceHealth(tt.instances)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// ---- helpers ----

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }
