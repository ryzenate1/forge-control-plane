package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Application struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	OrgID              string          `json:"orgId"`
	ProjectID          *string         `json:"projectId,omitempty"`
	EnvironmentID      *string         `json:"environmentId,omitempty"`
	ServerID           *string         `json:"serverId,omitempty"`
	SourceType         string          `json:"sourceType"`
	SourceConfig       json.RawMessage `json:"sourceConfig,omitempty"`
	DesiredState       string          `json:"desiredState"`
	ObservedStatus     string          `json:"observedStatus"`
	CurrentDeploymentID *string         `json:"currentDeploymentId,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type ServiceMode string

const (
	ServiceModeReplicated ServiceMode = "replicated"
	ServiceModeGlobal     ServiceMode = "global"
)

type UpdateConfig struct {
	Strategy      string `json:"strategy"`
	RollingOrder  string `json:"rollingOrder"`
	MonitorPeriod int    `json:"monitorPeriod"`
}

type HealthCheckConfig struct {
	Path     string `json:"path,omitempty"`
	Port     int    `json:"port,omitempty"`
	Interval int    `json:"interval,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
	Retries  int    `json:"retries,omitempty"`
}

type ResourceSpec struct {
	CPU      int `json:"cpu"`
	MemoryMB int `json:"memoryMb"`
	DiskMB   int `json:"diskMb"`
}

type VolumeRef struct {
	Name       string `json:"name"`
	Target     string `json:"target"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
	Type       string `json:"type,omitempty"`
}

type SecretRef struct {
	Name       string `json:"name"`
	Target     string `json:"target"`
	Key        string `json:"key,omitempty"`
}

type AppService struct {
	ID             string            `json:"id"`
	AppID          string            `json:"appId"`
	Name           string            `json:"name"`
	Image          string            `json:"image,omitempty"`
	ComposeService string            `json:"composeService,omitempty"`
	Replicas       int               `json:"replicas"`
	Ports          []AppPort         `json:"ports,omitempty"`
	EnvVars        map[string]string `json:"envVars,omitempty"`
	DependsOn      []string          `json:"dependsOn,omitempty"`
	DesiredState   string            `json:"desiredState"`
	ObservedStatus string            `json:"observedStatus"`
	Mode           string            `json:"mode,omitempty"`
	UpdateConfig   UpdateConfig      `json:"updateConfig,omitempty"`
	HealthCheck    HealthCheckConfig `json:"healthCheck,omitempty"`
	Resources      ResourceSpec      `json:"resources,omitempty"`
	Volumes        []VolumeRef       `json:"volumes,omitempty"`
	Secrets        []SecretRef       `json:"secrets,omitempty"`
	ReplicaAppID   *string           `json:"replicaAppId,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type UpdateAppServiceInput struct {
	Name           *string
	Image          *string
	Replicas       *int
	Ports          *[]AppPort
	EnvVars        *map[string]string
	DependsOn      *[]string
	DesiredState   *string
	Mode           *string
	UpdateConfig   *UpdateConfig
	HealthCheck    *HealthCheckConfig
	Resources      *ResourceSpec
	Volumes        *[]VolumeRef
	Secrets        *[]SecretRef
	ReplicaAppID   *string
}

type AppPort struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
}

type CreateApplicationInput struct {
	Name          string
	Description   string
	OrgID         string
	ProjectID     *string
	EnvironmentID *string
	ServerID      *string
	SourceType    string
	SourceConfig  json.RawMessage
}

type UpdateApplicationInput struct {
	Name          *string
	Description   *string
	ProjectID     *string
	EnvironmentID *string
	DesiredState  *string
	SourceConfig  json.RawMessage
}

type CreateAppServiceInput struct {
	AppID          string
	Name           string
	Image          string
	ComposeService string
	Replicas       int
	Ports          []AppPort
	EnvVars        map[string]string
	DependsOn      []string
	Mode           string
	UpdateConfig   UpdateConfig
	HealthCheck    HealthCheckConfig
	Resources      ResourceSpec
	Volumes        []VolumeRef
	Secrets        []SecretRef
}

func (s *Store) CreateApplication(ctx context.Context, input CreateApplicationInput) (*Application, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	desiredState := "running"

	configBytes := input.SourceConfig
	if configBytes == nil {
		configBytes = []byte("{}")
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO applications (id, name, description, org_id, project_id, environment_id, server_id,
			source_type, source_config, desired_state, observed_status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, id, input.Name, input.Description, input.OrgID,
		nullIfEmptyPtr(input.ProjectID), nullIfEmptyPtr(input.EnvironmentID), nullIfEmptyPtr(input.ServerID),
		input.SourceType, configBytes, desiredState, "idle", now, now)
	if err != nil {
		return nil, err
	}

	return s.GetApplication(ctx, id)
}

func (s *Store) GetApplication(ctx context.Context, id string) (*Application, error) {
	var a Application
	var projectID, envID, serverID, currentDeplID *string
	var configBytes []byte
	var description string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(description, ''), org_id::text,
			project_id::text, environment_id::text, server_id::text,
			source_type, source_config::text,
			desired_state, observed_status, current_deployment_id::text,
			created_at, updated_at
		FROM applications WHERE id = $1
	`, id).Scan(&a.ID, &a.Name, &description, &a.OrgID,
		&projectID, &envID, &serverID,
		&a.SourceType, &configBytes,
		&a.DesiredState, &a.ObservedStatus, &currentDeplID,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}

	a.Description = description
	a.ProjectID = projectID
	a.EnvironmentID = envID
	a.ServerID = serverID
	a.CurrentDeploymentID = currentDeplID
	if len(configBytes) > 0 {
		a.SourceConfig = json.RawMessage(configBytes)
	} else {
		a.SourceConfig = json.RawMessage("{}")
	}

	return &a, nil
}

func (s *Store) ListApplications(ctx context.Context, orgID string) ([]Application, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(description, ''), org_id::text,
			project_id::text, environment_id::text, server_id::text,
			source_type, COALESCE(source_config::text, '{}'),
			desired_state, observed_status, current_deployment_id::text,
			created_at, updated_at
		FROM applications
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Application
	for rows.Next() {
		var a Application
		var projectID, envID, serverID, currentDeplID *string
		var description string
		var configBytes []byte

		if err := rows.Scan(&a.ID, &a.Name, &description, &a.OrgID,
			&projectID, &envID, &serverID,
			&a.SourceType, &configBytes,
			&a.DesiredState, &a.ObservedStatus, &currentDeplID,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}

		a.Description = description
		a.ProjectID = projectID
		a.EnvironmentID = envID
		a.ServerID = serverID
		a.CurrentDeploymentID = currentDeplID
		if len(configBytes) > 0 {
			a.SourceConfig = json.RawMessage(configBytes)
		} else {
			a.SourceConfig = json.RawMessage("{}")
		}

		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Store) ListApplicationsByProject(ctx context.Context, projectID string) ([]Application, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(description, ''), org_id::text,
			project_id::text, environment_id::text, server_id::text,
			source_type, COALESCE(source_config::text, '{}'),
			desired_state, observed_status, current_deployment_id::text,
			created_at, updated_at
		FROM applications
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Application
	for rows.Next() {
		var a Application
		var projectID, envID, serverID, currentDeplID *string
		var description string
		var configBytes []byte

		if err := rows.Scan(&a.ID, &a.Name, &description, &a.OrgID,
			&projectID, &envID, &serverID,
			&a.SourceType, &configBytes,
			&a.DesiredState, &a.ObservedStatus, &currentDeplID,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}

		a.Description = description
		a.ProjectID = projectID
		a.EnvironmentID = envID
		a.ServerID = serverID
		a.CurrentDeploymentID = currentDeplID
		if len(configBytes) > 0 {
			a.SourceConfig = json.RawMessage(configBytes)
		} else {
			a.SourceConfig = json.RawMessage("{}")
		}

		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Store) UpdateApplication(ctx context.Context, id string, input UpdateApplicationInput) error {
	now := time.Now().UTC()

	// Build dynamic SET clause
	sets := []string{}
	args := []any{}
	argIdx := 1

	if input.Name != nil {
		sets = append(sets, "name = $"+intToStr(argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.Description != nil {
		sets = append(sets, "description = $"+intToStr(argIdx))
		args = append(args, *input.Description)
		argIdx++
	}
	if input.ProjectID != nil {
		sets = append(sets, "project_id = $"+intToStr(argIdx))
		args = append(args, nullIfEmptyPtr(input.ProjectID))
		argIdx++
	}
	if input.EnvironmentID != nil {
		sets = append(sets, "environment_id = $"+intToStr(argIdx))
		args = append(args, nullIfEmptyPtr(input.EnvironmentID))
		argIdx++
	}
	if input.DesiredState != nil {
		sets = append(sets, "desired_state = $"+intToStr(argIdx))
		args = append(args, *input.DesiredState)
		argIdx++
	}
	if input.SourceConfig != nil {
		sets = append(sets, "source_config = $"+intToStr(argIdx))
		args = append(args, input.SourceConfig)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = $"+intToStr(argIdx))
	args = append(args, now)
	argIdx++

	args = append(args, id)
	query := "UPDATE applications SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = $" + intToStr(argIdx)

	_, err := s.db.Exec(ctx, query, args...)
	return err
}

func (s *Store) DeleteApplication(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM applications WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateApplicationStatus(ctx context.Context, id string, observedStatus string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE applications SET observed_status = $2, updated_at = now()
		WHERE id = $1
	`, id, observedStatus)
	return err
}

func (s *Store) SetApplicationDeployment(ctx context.Context, appID string, deploymentID *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE applications SET current_deployment_id = $2, updated_at = now()
		WHERE id = $1
	`, appID, deploymentID)
	return err
}

// ---- App Services ----

func (s *Store) CreateAppService(ctx context.Context, input CreateAppServiceInput) (*AppService, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	portsJSON, _ := json.Marshal(input.Ports)
	envJSON, _ := json.Marshal(input.EnvVars)
	dependsJSON, _ := json.Marshal(input.DependsOn)
	mode := input.Mode
	if mode == "" {
		mode = "replicated"
	}
	ucJSON, _ := json.Marshal(input.UpdateConfig)
	hcJSON, _ := json.Marshal(input.HealthCheck)
	resJSON, _ := json.Marshal(input.Resources)
	volJSON, _ := json.Marshal(input.Volumes)
	secJSON, _ := json.Marshal(input.Secrets)

	_, err := s.db.Exec(ctx, `
		INSERT INTO app_services (id, app_id, name, image, compose_service, replicas,
			ports, env_vars, depends_on, desired_state, observed_status,
			mode, update_config, health_check, resources, volumes, secrets,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17,
			$18, $19)
	`, id, input.AppID, input.Name, input.Image, input.ComposeService,
		input.Replicas, portsJSON, envJSON, dependsJSON, "running", "idle",
		mode, ucJSON, hcJSON, resJSON, volJSON, secJSON,
		now, now)
	if err != nil {
		return nil, err
	}

	return s.GetAppService(ctx, id)
}

func (s *Store) GetAppService(ctx context.Context, id string) (*AppService, error) {
	var svc AppService
	var portsJSON, envJSON, dependsJSON []byte
	var ucJSON, hcJSON, resJSON, volJSON, secJSON []byte
	var replicaAppID *string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, app_id::text, name, COALESCE(image, ''), COALESCE(compose_service, ''),
			replicas, COALESCE(ports::text, '[]'), COALESCE(env_vars::text, '{}'),
			COALESCE(depends_on::text, '[]'), desired_state, observed_status,
			COALESCE(mode, 'replicated'), COALESCE(update_config::text, '{}'),
			COALESCE(health_check::text, '{}'), COALESCE(resources::text, '{}'),
			COALESCE(volumes::text, '[]'), COALESCE(secrets::text, '[]'),
			replica_app_id::text,
			created_at, updated_at
		FROM app_services WHERE id = $1
	`, id).Scan(&svc.ID, &svc.AppID, &svc.Name, &svc.Image, &svc.ComposeService,
		&svc.Replicas, &portsJSON, &envJSON, &dependsJSON,
		&svc.DesiredState, &svc.ObservedStatus,
		&svc.Mode, &ucJSON, &hcJSON, &resJSON, &volJSON, &secJSON,
		&replicaAppID,
		&svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(portsJSON, &svc.Ports)
	json.Unmarshal(envJSON, &svc.EnvVars)
	json.Unmarshal(dependsJSON, &svc.DependsOn)
	json.Unmarshal(ucJSON, &svc.UpdateConfig)
	json.Unmarshal(hcJSON, &svc.HealthCheck)
	json.Unmarshal(resJSON, &svc.Resources)
	json.Unmarshal(volJSON, &svc.Volumes)
	json.Unmarshal(secJSON, &svc.Secrets)
	svc.ReplicaAppID = replicaAppID

	return &svc, nil
}

func (s *Store) ListAppServices(ctx context.Context, appID string) ([]AppService, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, app_id::text, name, COALESCE(image, ''), COALESCE(compose_service, ''),
			replicas, COALESCE(ports::text, '[]'), COALESCE(env_vars::text, '{}'),
			COALESCE(depends_on::text, '[]'), desired_state, observed_status,
			COALESCE(mode, 'replicated'), COALESCE(update_config::text, '{}'),
			COALESCE(health_check::text, '{}'), COALESCE(resources::text, '{}'),
			COALESCE(volumes::text, '[]'), COALESCE(secrets::text, '[]'),
			replica_app_id::text,
			created_at, updated_at
		FROM app_services
		WHERE app_id = $1
		ORDER BY created_at ASC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AppService
	for rows.Next() {
		var svc AppService
		var portsJSON, envJSON, dependsJSON []byte
		var ucJSON, hcJSON, resJSON, volJSON, secJSON []byte
		var replicaAppID *string

		if err := rows.Scan(&svc.ID, &svc.AppID, &svc.Name, &svc.Image, &svc.ComposeService,
			&svc.Replicas, &portsJSON, &envJSON, &dependsJSON,
			&svc.DesiredState, &svc.ObservedStatus,
			&svc.Mode, &ucJSON, &hcJSON, &resJSON, &volJSON, &secJSON,
			&replicaAppID,
			&svc.CreatedAt, &svc.UpdatedAt); err != nil {
			return nil, err
		}

		json.Unmarshal(portsJSON, &svc.Ports)
		json.Unmarshal(envJSON, &svc.EnvVars)
		json.Unmarshal(dependsJSON, &svc.DependsOn)
		json.Unmarshal(ucJSON, &svc.UpdateConfig)
		json.Unmarshal(hcJSON, &svc.HealthCheck)
		json.Unmarshal(resJSON, &svc.Resources)
		json.Unmarshal(volJSON, &svc.Volumes)
		json.Unmarshal(secJSON, &svc.Secrets)
		svc.ReplicaAppID = replicaAppID

		result = append(result, svc)
	}
	return result, rows.Err()
}

func (s *Store) DeleteAppService(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM app_services WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateAppService(ctx context.Context, id string, input UpdateAppServiceInput) (*AppService, error) {
	now := time.Now().UTC()

	sets := []string{}
	args := []any{}
	argIdx := 1

	if input.Name != nil {
		sets = append(sets, "name = $"+intToStr(argIdx))
		args = append(args, *input.Name)
		argIdx++
	}
	if input.Image != nil {
		sets = append(sets, "image = $"+intToStr(argIdx))
		args = append(args, *input.Image)
		argIdx++
	}
	if input.Replicas != nil {
		sets = append(sets, "replicas = $"+intToStr(argIdx))
		args = append(args, *input.Replicas)
		argIdx++
	}
	if input.Ports != nil {
		b, _ := json.Marshal(*input.Ports)
		sets = append(sets, "ports = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.EnvVars != nil {
		b, _ := json.Marshal(*input.EnvVars)
		sets = append(sets, "env_vars = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.DependsOn != nil {
		b, _ := json.Marshal(*input.DependsOn)
		sets = append(sets, "depends_on = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.DesiredState != nil {
		sets = append(sets, "desired_state = $"+intToStr(argIdx))
		args = append(args, *input.DesiredState)
		argIdx++
	}
	if input.Mode != nil {
		sets = append(sets, "mode = $"+intToStr(argIdx))
		args = append(args, *input.Mode)
		argIdx++
	}
	if input.UpdateConfig != nil {
		b, _ := json.Marshal(*input.UpdateConfig)
		sets = append(sets, "update_config = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.HealthCheck != nil {
		b, _ := json.Marshal(*input.HealthCheck)
		sets = append(sets, "health_check = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.Resources != nil {
		b, _ := json.Marshal(*input.Resources)
		sets = append(sets, "resources = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.Volumes != nil {
		b, _ := json.Marshal(*input.Volumes)
		sets = append(sets, "volumes = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.Secrets != nil {
		b, _ := json.Marshal(*input.Secrets)
		sets = append(sets, "secrets = $"+intToStr(argIdx))
		args = append(args, b)
		argIdx++
	}
	if input.ReplicaAppID != nil {
		sets = append(sets, "replica_app_id = $"+intToStr(argIdx))
		args = append(args, *input.ReplicaAppID)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetAppService(ctx, id)
	}

	sets = append(sets, "updated_at = $"+intToStr(argIdx))
	args = append(args, now)
	argIdx++

	args = append(args, id)
	query := "UPDATE app_services SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = $" + intToStr(argIdx)

	_, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return s.GetAppService(ctx, id)
}

func (s *Store) UpdateAppServiceStatus(ctx context.Context, id string, observedStatus string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE app_services SET observed_status = $2, updated_at = now()
		WHERE id = $1
	`, id, observedStatus)
	return err
}

// ---- Service Endpoints ----

type ServiceEndpoint struct {
	ID         string    `json:"id"`
	ServiceID  string    `json:"serviceId"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Protocol   string    `json:"protocol"`
	NodeID     *string   `json:"nodeId,omitempty"`
	InstanceID *string   `json:"instanceId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *Store) CreateServiceEndpoint(ctx context.Context, serviceID, host string, port int, protocol string, nodeID, instanceID *string) (*ServiceEndpoint, error) {
	id := uuid.NewString()
	if protocol == "" {
		protocol = "tcp"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO service_endpoints (id, service_id, host, port, protocol, node_id, instance_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, serviceID, host, port, protocol, nodeID, instanceID)
	if err != nil {
		return nil, err
	}
	return s.GetServiceEndpoint(ctx, id)
}

func (s *Store) GetServiceEndpoint(ctx context.Context, id string) (*ServiceEndpoint, error) {
	var ep ServiceEndpoint
	err := s.db.QueryRow(ctx, `
		SELECT id::text, service_id::text, host, port, protocol, node_id::text, instance_id::text, created_at
		FROM service_endpoints WHERE id = $1
	`, id).Scan(&ep.ID, &ep.ServiceID, &ep.Host, &ep.Port, &ep.Protocol, &ep.NodeID, &ep.InstanceID, &ep.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func (s *Store) ListServiceEndpoints(ctx context.Context, serviceID string) ([]ServiceEndpoint, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, service_id::text, host, port, protocol, node_id::text, instance_id::text, created_at
		FROM service_endpoints WHERE service_id = $1 ORDER BY created_at
	`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ServiceEndpoint
	for rows.Next() {
		var ep ServiceEndpoint
		if err := rows.Scan(&ep.ID, &ep.ServiceID, &ep.Host, &ep.Port, &ep.Protocol, &ep.NodeID, &ep.InstanceID, &ep.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ep)
	}
	return result, rows.Err()
}

func (s *Store) DeleteServiceEndpoint(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM service_endpoints WHERE id = $1`, id)
	return err
}

func (s *Store) DeleteServiceEndpointsByService(ctx context.Context, serviceID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM service_endpoints WHERE service_id = $1`, serviceID)
	return err
}

// ---- Validation ----

func (s *Store) AppBelongsToOrg(ctx context.Context, appID, orgID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM applications WHERE id = $1 AND org_id = $2)
	`, appID, orgID).Scan(&exists)
	return exists, err
}

func (s *Store) AppServiceBelongsToApp(ctx context.Context, serviceID, appID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM app_services WHERE id = $1 AND app_id = $2)
	`, serviceID, appID).Scan(&exists)
	return exists, err
}

func nullIfEmptyPtr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
