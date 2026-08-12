package apphosting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	deployer   StackDeployer
}

// New builds the application-hosting service. The stack deployer is optional so
// that existing callers (and unit tests driving the store only) keep compiling;
// without it TriggerDeploy fails closed instead of recording an unexecuted
// deployment. cmd/api/main.go injects the compose lifecycle service.
func New(st Store, ts *tenancy.Service, deployers ...StackDeployer) *Service {
	svc := &Service{
		store:      st,
		tenancySvc: ts,
	}
	for _, deployer := range deployers {
		if deployer != nil {
			svc.deployer = deployer
			break
		}
	}
	return svc
}

// ---- Deploy execution ----

// StackDeployRequest describes one compose-stack materialisation of an
// application. It carries plain fields so this package does not depend on the
// compose service's own request types.
type StackDeployRequest struct {
	UserID        string
	Name          string
	NodeID        string
	ComposeYAML   string
	EnvVars       map[string]string
	MemoryMB      int64
	CPUShares     int64
	DiskMB        int64
	EnvironmentID string
}

// StackUpdateRequest is the in-place update of an already deployed stack.
type StackUpdateRequest struct {
	ComposeYAML string
	EnvVars     map[string]string
	MemoryMB    int64
	CPUShares   int64
	DiskMB      int64
}

// DeployedStack is the subset of stack state a deploy reports back.
type DeployedStack struct {
	ID     string
	Status string
	Error  string
}

// StackDeployer runs the real deployment. The compose lifecycle service
// satisfies it through the adapter wired in cmd/api/main.go.
type StackDeployer interface {
	DeployStack(ctx context.Context, req StackDeployRequest) (DeployedStack, error)
	UpdateStack(ctx context.Context, stackID string, req StackUpdateRequest) (DeployedStack, error)
}

// DeploymentStatusStore is the optional store capability TriggerDeploy uses to
// reconcile the deployment row with the outcome of the real deploy.
// *store.Store implements it.
type DeploymentStatusStore interface {
	UpdateDeploymentStatus(ctx context.Context, id string, status string, errMsg string) error
}

// ServerStore is the optional store capability that resolves the server an
// application is bound to: its owner owns the compose stack and its node runs
// it. *store.Store implements it.
type ServerStore interface {
	GetServer(ctx context.Context, id string) (store.Server, error)
}

// appSourceSpec is the subset of applications.source_config the deploy path
// reads. The compose editor screen stores the document under "content" (the
// creation form uses "composeContent") and TriggerDeploy remembers the deployed
// stack id under "stackId" so the next deploy updates that stack in place
// instead of leaking a second one.
type appSourceSpec struct {
	Content        string            `json:"content"`
	ComposeContent string            `json:"composeContent"`
	StackID        string            `json:"stackId"`
	NodeID         string            `json:"nodeId"`
	UserID         string            `json:"userId"`
	Image          string            `json:"image"`
	MemoryMB       int64             `json:"memoryMb"`
	CPUShares      int64             `json:"cpuShares"`
	DiskMB         int64             `json:"diskMb"`
	EnvVars        map[string]string `json:"envVars"`
}

func (s appSourceSpec) composeDocument() string {
	if strings.TrimSpace(s.Content) != "" {
		return s.Content
	}
	return s.ComposeContent
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
	// Flat aliases accepted by the web creation wizard. When SourceConfig is
	// absent they are folded into it; an explicit SourceConfig always wins.
	// Type accepts the wizard values (image, git, compose) or the canonical
	// uppercase values (DOCKER_IMAGE, GIT, COMPOSE).
	Type           string `json:"type"`
	Image          string `json:"image"`
	GitURL         string `json:"gitUrl"`
	GitBranch      string `json:"gitBranch"`
	GitProvider    string `json:"gitProvider"`
	ComposeContent string `json:"composeContent"`
	NodeID         string `json:"nodeId"`
	RegionID       string `json:"regionId"`
}

type UpdateAppRequest struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	ProjectID     *string         `json:"projectId,omitempty"`
	EnvironmentID *string         `json:"environmentId,omitempty"`
	DesiredState  *string         `json:"desiredState,omitempty"`
	SourceConfig  json.RawMessage `json:"sourceConfig,omitempty"`
	// Flat aliases accepted by the web editor. Each non-nil field is merged
	// into the stored source_config; an explicit SourceConfig replaces it
	// wholesale as before.
	Image          *string           `json:"image,omitempty"`
	GitBranch      *string           `json:"gitBranch,omitempty"`
	ComposeContent *string           `json:"composeContent,omitempty"`
	EnvVars        map[string]string `json:"envVars,omitempty"`
	Ports          []store.AppPort   `json:"ports,omitempty"`
	Volumes        []any             `json:"volumes,omitempty"`
	CPULimit       *string           `json:"cpuLimit,omitempty"`
	MemoryLimit    *string           `json:"memoryLimit,omitempty"`
	DiskLimit      *string           `json:"diskLimit,omitempty"`
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

// wizardSourceTypes maps the web creation wizard's lowercase type values to
// the canonical uppercase source types stored in the database.
var wizardSourceTypes = map[string]string{
	"image":          "DOCKER_IMAGE",
	"docker_image":   "DOCKER_IMAGE",
	"docker-image":   "DOCKER_IMAGE",
	"git":            "GIT",
	"compose":        "COMPOSE",
	"docker_compose": "COMPOSE",
	"docker-compose": "COMPOSE",
}

func resolveSourceType(sourceType, typeAlias string) string {
	if st := strings.ToUpper(strings.TrimSpace(sourceType)); st != "" {
		return st
	}
	if st, ok := wizardSourceTypes[strings.ToLower(strings.TrimSpace(typeAlias))]; ok {
		return st
	}
	return strings.ToUpper(strings.TrimSpace(typeAlias))
}

// synthesizeSourceConfig folds the wizard's flat fields into the source_config
// document the deploy path reads (see appSourceSpec). It returns nil when no
// flat field carries a value so callers can tell "nothing to fold" apart from
// "explicitly empty".
func synthesizeSourceConfig(image, gitURL, gitBranch, gitProvider, composeContent, nodeID, regionID string) json.RawMessage {
	spec := map[string]any{}
	if v := strings.TrimSpace(image); v != "" {
		spec["image"] = v
	}
	if v := strings.TrimSpace(gitURL); v != "" {
		spec["gitUrl"] = v
	}
	if v := strings.TrimSpace(gitBranch); v != "" {
		spec["gitBranch"] = v
	}
	if v := strings.TrimSpace(gitProvider); v != "" {
		spec["gitProvider"] = v
	}
	if v := strings.TrimSpace(composeContent); v != "" {
		spec["composeContent"] = v
	}
	if v := strings.TrimSpace(nodeID); v != "" {
		spec["nodeId"] = v
	}
	if v := strings.TrimSpace(regionID); v != "" {
		spec["regionId"] = v
	}
	if len(spec) == 0 {
		return nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil
	}
	return raw
}

func sourceConfigIsEmpty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(raw) == 0 || trimmed == "" || trimmed == "{}" || trimmed == "null"
}

// mergeSourceConfig overlays flat editor fields onto the stored source_config
// document and returns the merged document. Unknown existing keys survive.
func mergeSourceConfig(current json.RawMessage, updates map[string]any) (json.RawMessage, error) {
	merged := map[string]any{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &merged); err != nil {
			return nil, fmt.Errorf("stored source config is unreadable: %w", err)
		}
		if merged == nil {
			merged = map[string]any{}
		}
	}
	for k, v := range updates {
		merged[k] = v
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged source config: %w", err)
	}
	return raw, nil
}

// parseResourceNumber parses a UI resource string ("1024", "1.5") into an
// integer for the deploy path. It reports an error instead of silently
// storing zero.
func parseResourceNumber(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("empty resource value")
	}
	var f float64
	var err error
	if f, err = strconv.ParseFloat(trimmed, 64); err != nil || f < 0 {
		return 0, errors.New("resource value must be a non-negative number")
	}
	return int64(f), nil
}

func (svc *Service) CreateApp(ctx context.Context, claims tenancy.OrgContext, req CreateAppRequest) (*store.Application, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	sourceType := resolveSourceType(req.SourceType, req.Type)
	if sourceType == "" {
		sourceType = "DOCKER_IMAGE"
	}
	if !validSourceTypes[sourceType] {
		return nil, errors.New("invalid source_type: must be GIT, DOCKER_IMAGE, or COMPOSE")
	}

	if sourceConfigIsEmpty(req.SourceConfig) {
		if built := synthesizeSourceConfig(req.Image, req.GitURL, req.GitBranch, req.GitProvider, req.ComposeContent, req.NodeID, req.RegionID); built != nil {
			req.SourceConfig = built
		}
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

	// Fold flat editor fields into the stored source_config so partial edits
	// (image, env, ports, volumes, limits) persist instead of being dropped.
	// An explicit SourceConfig still replaces the document wholesale.
	if req.SourceConfig == nil {
		updates := map[string]any{}
		if req.Image != nil {
			updates["image"] = strings.TrimSpace(*req.Image)
		}
		if req.GitBranch != nil {
			updates["gitBranch"] = strings.TrimSpace(*req.GitBranch)
		}
		if req.ComposeContent != nil {
			updates["composeContent"] = *req.ComposeContent
		}
		if req.EnvVars != nil {
			updates["envVars"] = req.EnvVars
		}
		if req.Ports != nil {
			updates["ports"] = req.Ports
		}
		if req.Volumes != nil {
			updates["volumes"] = req.Volumes
		}
		if req.CPULimit != nil {
			updates["cpuLimit"] = strings.TrimSpace(*req.CPULimit)
		}
		if req.MemoryLimit != nil {
			updates["memoryLimit"] = strings.TrimSpace(*req.MemoryLimit)
			if n, err := parseResourceNumber(*req.MemoryLimit); err == nil {
				updates["memoryMb"] = n
			}
		}
		if req.DiskLimit != nil {
			updates["diskLimit"] = strings.TrimSpace(*req.DiskLimit)
			if n, err := parseResourceNumber(*req.DiskLimit); err == nil {
				updates["diskMb"] = n
			}
		}
		if len(updates) > 0 {
			current, err := svc.store.GetApplication(ctx, appID)
			if err != nil {
				return nil, err
			}
			merged, err := mergeSourceConfig(current.SourceConfig, updates)
			if err != nil {
				return nil, err
			}
			input.SourceConfig = merged
		}
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

// TriggerDeploy runs a real deployment of the application's compose document
// onto the node its bound server occupies, and records the attempt as a
// deployment row whose status reflects what the node actually reported. It never
// returns a deployment that nothing executed: without a deployer, without a
// document, or without a resolvable owner and node it fails closed.
//
// A stack already recorded in the application's source config is updated in
// place, so repeated deploys do not accumulate orphaned stacks on the node.
func (svc *Service) TriggerDeploy(ctx context.Context, appID, orgID string) (*store.Deployment, error) {
	belongs, err := svc.store.AppBelongsToOrg(ctx, appID, orgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("application not found")
	}

	if svc.deployer == nil {
		return nil, errors.New("app-hosting deploy is not wired to a runtime engine; deploy the bound workload instead")
	}

	app, err := svc.store.GetApplication(ctx, appID)
	if err != nil {
		return nil, err
	}

	var spec appSourceSpec
	if len(app.SourceConfig) > 0 {
		if err := json.Unmarshal(app.SourceConfig, &spec); err != nil {
			return nil, fmt.Errorf("application source config is unreadable: %w", err)
		}
	}
	composeDocument := strings.TrimSpace(spec.composeDocument())
	if composeDocument == "" {
		return nil, errors.New("application has no compose document to deploy")
	}
	if app.ServerID == nil || strings.TrimSpace(*app.ServerID) == "" {
		return nil, errors.New("application is not bound to a server that can run it")
	}
	serverStore, ok := svc.store.(ServerStore)
	if !ok {
		return nil, errors.New("application store cannot resolve the bound server")
	}
	server, err := serverStore.GetServer(ctx, *app.ServerID)
	if err != nil {
		return nil, fmt.Errorf("load bound server: %w", err)
	}

	// The stack is owned by whoever owns the server and runs where that server
	// runs. Values recorded on the application win, so a deployment pinned to a
	// specific node or account is not silently re-placed.
	userID := strings.TrimSpace(spec.UserID)
	if userID == "" {
		userID = server.OwnerID
	}
	nodeID := strings.TrimSpace(spec.NodeID)
	if nodeID == "" {
		nodeID = server.NodeID
	}
	if nodeID == "" {
		nodeID = server.Node
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(nodeID) == "" {
		return nil, errors.New("bound server has no owner or node to deploy onto")
	}

	now := time.Now().UTC()
	deployment := &store.Deployment{
		ID:        uuid.NewString(),
		ServerID:  server.ID,
		Strategy:  "recreate",
		Status:    "pending",
		Image:     spec.Image,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := svc.store.CreateDeployment(ctx, deployment); err != nil {
		return nil, fmt.Errorf("record deployment: %w", err)
	}
	if err := svc.store.SetApplicationDeployment(ctx, appID, &deployment.ID); err != nil {
		// The deployment exists whether or not the application can be pointed at
		// it; say so instead of pretending the record was linked.
		return nil, fmt.Errorf("link deployment to application: %w", err)
	}

	stack, deployErr := svc.deployExistingOrNewStack(ctx, spec, deployment, userID, nodeID, app, composeDocument)
	if statusStore, ok := svc.store.(DeploymentStatusStore); ok {
		status := "completed"
		errorText := ""
		if deployErr != nil {
			status = "failed"
			errorText = deployErr.Error()
		} else if stack.Status != "" {
			// The node's own word for the stack beats a locally assumed success.
			status = stack.Status
		}
		if err := statusStore.UpdateDeploymentStatus(ctx, deployment.ID, status, errorText); err != nil {
			return nil, fmt.Errorf("record deployment outcome %q: %w", status, err)
		}
	}
	if deployErr != nil {
		return nil, deployErr
	}

	// Observed status is what the node reported, not a locally chosen label.
	if stack.Status != "" {
		if err := svc.store.UpdateApplicationStatus(ctx, appID, stack.Status); err != nil {
			return nil, fmt.Errorf("record application status: %w", err)
		}
	}
	if fresh := deploymentForStatus(deployment, stack.Status); fresh != nil {
		return fresh, nil
	}
	return deployment, nil
}

// deployExistingOrNewStack updates the stack recorded on the application, or
// creates one, and remembers the new stack id on the application so the next
// deploy updates it in place rather than leaking a second stack onto the node.
func (svc *Service) deployExistingOrNewStack(ctx context.Context, spec appSourceSpec, deployment *store.Deployment, userID, nodeID string, app *store.Application, composeDocument string) (DeployedStack, error) {
	environmentID := ""
	if app.EnvironmentID != nil {
		environmentID = *app.EnvironmentID
	}

	if existing := strings.TrimSpace(spec.StackID); existing != "" {
		stack, err := svc.deployer.UpdateStack(ctx, existing, StackUpdateRequest{
			ComposeYAML: composeDocument,
			EnvVars:     spec.EnvVars,
			MemoryMB:    spec.MemoryMB,
			CPUShares:   spec.CPUShares,
			DiskMB:      spec.DiskMB,
		})
		return stack, err
	}

	stack, err := svc.deployer.DeployStack(ctx, StackDeployRequest{
		UserID:        userID,
		Name:          fmt.Sprintf("app-%s", app.ID),
		NodeID:        nodeID,
		ComposeYAML:   composeDocument,
		EnvVars:       spec.EnvVars,
		MemoryMB:      spec.MemoryMB,
		CPUShares:     spec.CPUShares,
		DiskMB:        spec.DiskMB,
		EnvironmentID: environmentID,
	})
	if err != nil {
		return stack, err
	}
	if stack.ID == "" {
		return stack, errors.New("deploy reported success without a stack id")
	}
	// Persist the binding. Without this the next deploy creates a second stack
	// and the first one keeps running, unreferenced, on the node.
	spec.StackID = stack.ID
	if encoded, marshalErr := json.Marshal(spec); marshalErr == nil {
		if updateErr := svc.store.UpdateApplication(ctx, app.ID, store.UpdateApplicationInput{SourceConfig: encoded}); updateErr != nil {
			return stack, fmt.Errorf("deployed stack %s but could not record it on the application: %w", stack.ID, updateErr)
		}
	}
	return stack, nil
}

// deploymentForStatus reflects the reported outcome on the returned row so a
// caller reading it sees what the node said rather than the initial "pending".
func deploymentForStatus(deployment *store.Deployment, status string) *store.Deployment {
	if status == "" {
		return nil
	}
	deployment.Status = status
	return deployment
}
