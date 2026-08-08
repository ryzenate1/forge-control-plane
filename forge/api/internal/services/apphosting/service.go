package apphosting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type Store interface {
	CreateApplication(ctx context.Context, input store.CreateApplicationInput) (*store.Application, error)
	GetApplication(ctx context.Context, id string) (*store.Application, error)
	ListApplications(ctx context.Context, orgID string) ([]store.Application, error)
	UpdateApplication(ctx context.Context, id string, input store.UpdateApplicationInput) error
	DeleteApplication(ctx context.Context, id string) error
	UpdateApplicationStatus(ctx context.Context, id string, status string) error
	SetApplicationDeployment(ctx context.Context, appID string, deploymentID *string) error
	AppBelongsToOrg(ctx context.Context, appID, orgID string) (bool, error)
	CreateAppService(ctx context.Context, input store.CreateAppServiceInput) (*store.AppService, error)
	GetAppService(ctx context.Context, id string) (*store.AppService, error)
	ListAppServices(ctx context.Context, appID string) ([]store.AppService, error)
	UpdateAppService(ctx context.Context, id string, input store.UpdateAppServiceInput) (*store.AppService, error)
	DeleteAppService(ctx context.Context, id string) error
	AppServiceBelongsToApp(ctx context.Context, serviceID, appID string) (bool, error)
	CreateDeployment(ctx context.Context, d *store.Deployment) error
	ListInstancesByApp(ctx context.Context, appID string) ([]store.Instance, error)
	UpdateReplicaAppReplicas(ctx context.Context, appID string, replicas int) (store.ReplicaApplication, error)
	ListServiceEndpoints(ctx context.Context, serviceID string) ([]store.ServiceEndpoint, error)
}

type Service struct {
	store      Store
	tenancySvc *tenancy.Service
}

func New(st Store, ts *tenancy.Service) *Service {
	return &Service{
		store:      st,
		tenancySvc: ts,
	}
}

type CreateAppRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	OrgID         string          `json:"orgId"`
	ProjectID     *string         `json:"projectId,omitempty"`
	EnvironmentID *string         `json:"environmentId,omitempty"`
	ServerID      *string         `json:"serverId,omitempty"`
	SourceType    string          `json:"sourceType"`
	SourceConfig  json.RawMessage `json:"sourceConfig,omitempty"`
}

type UpdateAppRequest struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	ProjectID     *string         `json:"projectId,omitempty"`
	EnvironmentID *string         `json:"environmentId,omitempty"`
	DesiredState  *string         `json:"desiredState,omitempty"`
	SourceConfig  json.RawMessage `json:"sourceConfig,omitempty"`
}

type CreateServiceRequest struct {
	Name           string            `json:"name"`
	Image          string            `json:"image,omitempty"`
	ComposeService string            `json:"composeService,omitempty"`
	Replicas       int               `json:"replicas"`
	Ports          []store.AppPort   `json:"ports,omitempty"`
	EnvVars        map[string]string `json:"envVars,omitempty"`
	DependsOn      []string          `json:"dependsOn,omitempty"`
}

var validSourceTypes = map[string]bool{
	"GIT":          true,
	"DOCKER_IMAGE": true,
	"COMPOSE":      true,
}

var validDesiredStates = map[string]bool{
	"running": true,
	"stopped": true,
	"removed": true,
}

func (svc *Service) CreateApp(ctx context.Context, claims tenancy.OrgContext, req CreateAppRequest) (*store.Application, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	sourceType := strings.ToUpper(strings.TrimSpace(req.SourceType))
	if sourceType == "" {
		sourceType = "DOCKER_IMAGE"
	}
	if !validSourceTypes[sourceType] {
		return nil, errors.New("invalid source_type: must be GIT, DOCKER_IMAGE, or COMPOSE")
	}

	if req.SourceConfig == nil {
		req.SourceConfig = json.RawMessage("{}")
	}

	input := store.CreateApplicationInput{
		Name:          name,
		Description:   strings.TrimSpace(req.Description),
		OrgID:         req.OrgID,
		ProjectID:     req.ProjectID,
		EnvironmentID: req.EnvironmentID,
		ServerID:      req.ServerID,
		SourceType:    sourceType,
		SourceConfig:  req.SourceConfig,
	}

	return svc.store.CreateApplication(ctx, input)
}

func (svc *Service) GetApp(ctx context.Context, appID, orgID string) (*store.Application, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}
	return svc.store.GetApplication(ctx, appID)
}

func (svc *Service) ListApps(ctx context.Context, orgID string) ([]store.Application, error) {
	return svc.store.ListApplications(ctx, orgID)
}

func (svc *Service) UpdateApp(ctx context.Context, appID, orgID string, req UpdateAppRequest) (*store.Application, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	input := store.UpdateApplicationInput{
		SourceConfig: req.SourceConfig,
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		input.Name = &trimmed
	}
	input.Description = req.Description
	input.ProjectID = req.ProjectID
	input.EnvironmentID = req.EnvironmentID
	if req.DesiredState != nil {
		ds := strings.ToLower(strings.TrimSpace(*req.DesiredState))
		if !validDesiredStates[ds] {
			return nil, errors.New("invalid desired_state: must be running, stopped, or removed")
		}
		input.DesiredState = &ds
	}

	if err := svc.store.UpdateApplication(ctx, appID, input); err != nil {
		return nil, err
	}

	return svc.store.GetApplication(ctx, appID)
}

func (svc *Service) DeleteApp(ctx context.Context, appID, orgID string) error {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("application not found")
	}
	return svc.store.DeleteApplication(ctx, appID)
}

func (svc *Service) UpdateAppStatus(ctx context.Context, appID, orgID, status string) error {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("application not found")
	}
	return svc.store.UpdateApplicationStatus(ctx, appID, status)
}

// ---- Services ----

func (svc *Service) CreateService(ctx context.Context, appID, orgID string, req CreateServiceRequest) (*store.AppService, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("service name is required")
	}

	if req.Ports == nil {
		req.Ports = []store.AppPort{}
	}
	if req.EnvVars == nil {
		req.EnvVars = map[string]string{}
	}
	if req.DependsOn == nil {
		req.DependsOn = []string{}
	}

	input := store.CreateAppServiceInput{
		AppID:          appID,
		Name:           name,
		Image:          strings.TrimSpace(req.Image),
		ComposeService: strings.TrimSpace(req.ComposeService),
		Replicas:       req.Replicas,
		Ports:          req.Ports,
		EnvVars:        req.EnvVars,
		DependsOn:      req.DependsOn,
	}

	if input.Replicas < 1 {
		input.Replicas = 1
	}

	return svc.store.CreateAppService(ctx, input)
}

func (svc *Service) ListServices(ctx context.Context, appID, orgID string) ([]store.AppService, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}
	return svc.store.ListAppServices(ctx, appID)
}

func (svc *Service) DeleteService(ctx context.Context, serviceID, appID, orgID string) error {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return err
	}
	if !belongs {
		return errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return err
	}
	if !svcBelongs {
		return errors.New("service not found")
	}

	return svc.store.DeleteAppService(ctx, serviceID)
}

// ---- Uncloud-inspired Service Operations ----

type UpdateServiceRequest struct {
	Name         *string                  `json:"name,omitempty"`
	Image        *string                  `json:"image,omitempty"`
	Replicas     *int                     `json:"replicas,omitempty"`
	Ports        *[]store.AppPort         `json:"ports,omitempty"`
	EnvVars      *map[string]string       `json:"envVars,omitempty"`
	DependsOn    *[]string                `json:"dependsOn,omitempty"`
	DesiredState *string                  `json:"desiredState,omitempty"`
	Mode         *string                  `json:"mode,omitempty"`
	UpdateConfig *store.UpdateConfig      `json:"updateConfig,omitempty"`
	HealthCheck  *store.HealthCheckConfig `json:"healthCheck,omitempty"`
	Resources    *store.ResourceSpec      `json:"resources,omitempty"`
	Volumes      *[]store.VolumeRef       `json:"volumes,omitempty"`
	Secrets      *[]store.SecretRef       `json:"secrets,omitempty"`
}

type ServiceInstanceStatus struct {
	InstanceID string `json:"instanceId"`
	Index      int    `json:"index"`
	NodeID     string `json:"nodeId"`
	NodeName   string `json:"nodeName,omitempty"`
	Status     string `json:"status"`
	CPU        int    `json:"cpu"`
	MemoryMB   int    `json:"memoryMb"`
	DiskMB     int    `json:"diskMb"`
}

type ServiceStatusView struct {
	Service   *store.AppService       `json:"service"`
	Instances []ServiceInstanceStatus `json:"instances"`
	Endpoints []store.ServiceEndpoint `json:"endpoints"`
	Health    string                  `json:"health"`
}

type ServiceOverview struct {
	ID          string `json:"id"`
	AppID       string `json:"appId"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	Mode        string `json:"mode"`
	Replicas    int    `json:"replicas"`
	Running     int    `json:"running"`
	Desired     int    `json:"desired"`
	Status      string `json:"status"`
	Health      string `json:"health"`
	HasEndpoint bool   `json:"hasEndpoint"`
}

// ComputeServiceHealth aggregates instance statuses into a service-level health string.
func ComputeServiceHealth(instances []ServiceInstanceStatus) string {
	if len(instances) == 0 {
		return "unknown"
	}
	running := 0
	failed := 0
	total := 0
	for _, inst := range instances {
		if inst.Status == "removing" || inst.Status == "removed" {
			continue
		}
		total++
		switch inst.Status {
		case "running":
			running++
		case "failed":
			failed++
		}
	}
	if total == 0 {
		return "partial"
	}
	if failed == total {
		return "unhealthy"
	}
	if running == total {
		return "healthy"
	}
	if failed > 0 {
		return "degraded"
	}
	if running > 0 {
		return "partial"
	}
	return "unhealthy"
}

func (svc *Service) UpdateService(ctx context.Context, serviceID, appID, orgID string, req UpdateServiceRequest) (*store.AppService, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return nil, err
	}
	if !svcBelongs {
		return nil, errors.New("service not found")
	}

	input := store.UpdateAppServiceInput{}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		input.Name = &trimmed
	}
	input.Image = req.Image
	input.Replicas = req.Replicas
	input.Ports = req.Ports
	input.EnvVars = req.EnvVars
	input.DependsOn = req.DependsOn
	input.DesiredState = req.DesiredState
	input.Mode = req.Mode
	input.UpdateConfig = req.UpdateConfig
	input.HealthCheck = req.HealthCheck
	input.Resources = req.Resources
	input.Volumes = req.Volumes
	input.Secrets = req.Secrets

	return svc.store.UpdateAppService(ctx, serviceID, input)
}

func (svc *Service) ScaleService(ctx context.Context, serviceID, appID, orgID string, targetReplicas int) (*store.AppService, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return nil, err
	}
	if !svcBelongs {
		return nil, errors.New("service not found")
	}

	if targetReplicas < 0 {
		return nil, errors.New("target replicas must be non-negative")
	}
	if targetReplicas == 0 {
		targetReplicas = 0 // allow zero for stopped services
	}

	input := store.UpdateAppServiceInput{
		Replicas: &targetReplicas,
	}

	updated, err := svc.store.UpdateAppService(ctx, serviceID, input)
	if err != nil {
		return nil, fmt.Errorf("scale service: %w", err)
	}

	if updated.ReplicaAppID != nil {
		_, err = svc.store.UpdateReplicaAppReplicas(ctx, *updated.ReplicaAppID, targetReplicas)
		if err != nil {
			return updated, fmt.Errorf("service scaled but replica app update failed (non-fatal): %w", err)
		}
	}

	return updated, nil
}

func (svc *Service) GetServiceStatus(ctx context.Context, serviceID, appID, orgID string) (*ServiceStatusView, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return nil, err
	}
	if !svcBelongs {
		return nil, errors.New("service not found")
	}

	svcRec, err := svc.store.GetAppService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	endpoints, err := svc.store.ListServiceEndpoints(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	view := &ServiceStatusView{
		Service:   svcRec,
		Endpoints: endpoints,
		Instances: []ServiceInstanceStatus{},
	}

	if svcRec.ReplicaAppID != nil {
		insts, err := svc.store.ListInstancesByApp(ctx, *svcRec.ReplicaAppID)
		if err != nil {
			return view, fmt.Errorf("service loaded but instances unavailable: %w", err)
		}
		for _, inst := range insts {
			view.Instances = append(view.Instances, ServiceInstanceStatus{
				InstanceID: inst.ID,
				Index:      inst.Idx,
				NodeID:     inst.NodeID,
				Status:     inst.Status,
				CPU:        inst.CPU,
				MemoryMB:   inst.MemoryMB,
				DiskMB:     inst.DiskMB,
			})
		}
	}

	view.Health = ComputeServiceHealth(view.Instances)
	return view, nil
}

func (svc *Service) GetServiceEndpoints(ctx context.Context, serviceID, appID, orgID string) ([]store.ServiceEndpoint, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return nil, err
	}
	if !svcBelongs {
		return nil, errors.New("service not found")
	}

	return svc.store.ListServiceEndpoints(ctx, serviceID)
}

func (svc *Service) GetServiceOverview(ctx context.Context, appID, orgID string) ([]ServiceOverview, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	services, err := svc.store.ListAppServices(ctx, appID)
	if err != nil {
		return nil, err
	}

	var overviews []ServiceOverview
	for _, s := range services {
		o := ServiceOverview{
			ID:     s.ID,
			AppID:  s.AppID,
			Name:   s.Name,
			Image:  s.Image,
			Mode:   s.Mode,
			Status: s.ObservedStatus,
		}
		if o.Mode == "" {
			o.Mode = "replicated"
		}
		o.Replicas = s.Replicas
		o.Desired = s.Replicas

		if s.ReplicaAppID != nil {
			insts, err := svc.store.ListInstancesByApp(ctx, *s.ReplicaAppID)
			if err == nil {
				for _, inst := range insts {
					if inst.Status == "running" {
						o.Running++
					}
				}
			}
		}

		eps, err := svc.store.ListServiceEndpoints(ctx, s.ID)
		if err == nil && len(eps) > 0 {
			o.HasEndpoint = true
		}

		instStatuses := []ServiceInstanceStatus{}
		if s.ReplicaAppID != nil {
			insts, err := svc.store.ListInstancesByApp(ctx, *s.ReplicaAppID)
			if err == nil {
				for _, inst := range insts {
					instStatuses = append(instStatuses, ServiceInstanceStatus{
						Status: inst.Status,
					})
				}
			}
		}
		o.Health = ComputeServiceHealth(instStatuses)

		overviews = append(overviews, o)
	}

	return overviews, nil
}

// ---- Plan/Apply (diff-based update) ----

type ServicePlan struct {
	ServiceID       string              `json:"serviceId"`
	DesiredImage    string              `json:"desiredImage"`
	DesiredReplicas int                 `json:"desiredReplicas"`
	Changes         []ServicePlanChange `json:"changes"`
}

type ServicePlanChange struct {
	Field    string `json:"field"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

func (svc *Service) PlanServiceUpdate(ctx context.Context, serviceID, appID, orgID string, req UpdateServiceRequest) (*ServicePlan, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	svcBelongs, err := svc.store.AppServiceBelongsToApp(ctx, serviceID, appID)
	if err != nil {
		return nil, err
	}
	if !svcBelongs {
		return nil, errors.New("service not found")
	}

	current, err := svc.store.GetAppService(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	plan := &ServicePlan{
		ServiceID: serviceID,
	}

	if req.Image != nil && *req.Image != current.Image {
		plan.DesiredImage = *req.Image
		plan.Changes = append(plan.Changes, ServicePlanChange{
			Field:    "image",
			OldValue: current.Image,
			NewValue: *req.Image,
		})
	}
	if req.Replicas != nil && *req.Replicas != current.Replicas {
		plan.DesiredReplicas = *req.Replicas
		plan.Changes = append(plan.Changes, ServicePlanChange{
			Field:    "replicas",
			OldValue: fmt.Sprintf("%d", current.Replicas),
			NewValue: fmt.Sprintf("%d", *req.Replicas),
		})
	}
	if req.Mode != nil && *req.Mode != current.Mode {
		plan.Changes = append(plan.Changes, ServicePlanChange{
			Field:    "mode",
			OldValue: current.Mode,
			NewValue: *req.Mode,
		})
	}

	if plan.DesiredReplicas == 0 {
		plan.DesiredReplicas = current.Replicas
	}
	if plan.DesiredImage == "" {
		plan.DesiredImage = current.Image
	}

	return plan, nil
}

func (svc *Service) ApplyServiceUpdate(ctx context.Context, serviceID, appID, orgID string, plan *ServicePlan) (*store.AppService, error) {
	if plan == nil {
		return nil, errors.New("plan must not be nil")
	}

	req := UpdateServiceRequest{
		Image:    &plan.DesiredImage,
		Replicas: &plan.DesiredReplicas,
	}
	return svc.UpdateService(ctx, serviceID, appID, orgID, req)
}

func (svc *Service) TriggerDeploy(ctx context.Context, appID, orgID string) (*store.Deployment, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	app, err := svc.store.GetApplication(ctx, appID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	depl := &store.Deployment{
		ID:        uuid.NewString(),
		ServerID:  "",
		Strategy:  "recreate",
		Status:    "pending",
		Image:     "",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if app.ServerID != nil && *app.ServerID != "" {
		depl.ServerID = *app.ServerID
	}

	if err := svc.store.CreateDeployment(ctx, depl); err != nil {
		return nil, err
	}

	depID := depl.ID
	if err := svc.store.SetApplicationDeployment(ctx, appID, &depID); err != nil {
		return nil, err
	}

	return depl, nil
}
