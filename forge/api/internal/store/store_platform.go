package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/platform/tenancy"
	platformworkloads "gamepanel/forge/internal/platform/workloads"

	"github.com/google/uuid"
)

type CreateOrganizationRequest struct {
	Name string
	Slug string
}

type CreateProjectRequest struct {
	OrganizationID string
	Name           string
	Slug           string
}

type CreateEnvironmentRequest struct {
	ProjectID  string
	Name       string
	Slug       string
	Production bool
}

type CreateWorkloadRequest struct {
	EnvironmentID string
	Kind          platformworkloads.Kind
	Name          string
	DesiredState  platformworkloads.DesiredState
	Spec          json.RawMessage
	CreatedBy     string
}

type AgentCommand struct {
	ID              string          `json:"id"`
	NodeID          string          `json:"nodeId"`
	WorkloadID      string          `json:"workloadId,omitempty"`
	OperationID     string          `json:"operationId,omitempty"`
	CommandType     string          `json:"commandType"`
	ProtocolVersion int             `json:"protocolVersion"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	Payload         json.RawMessage `json:"payload"`
	Status          string          `json:"status"`
	Acknowledgement json.RawMessage `json:"acknowledgement,omitempty"`
	Error           string          `json:"error,omitempty"`
	IssuedAt        time.Time       `json:"issuedAt"`
	AcknowledgedAt  *time.Time      `json:"acknowledgedAt,omitempty"`
	CompletedAt     *time.Time      `json:"completedAt,omitempty"`
}

type CreateAgentCommandRequest struct {
	NodeID          string
	WorkloadID      string
	OperationID     string
	CommandType     string
	ProtocolVersion int
	IdempotencyKey  string
	Payload         json.RawMessage
}

func (s *Store) CreateOrganization(ctx context.Context, request CreateOrganizationRequest) (tenancy.Organization, error) {
	name, slug := strings.TrimSpace(request.Name), strings.ToLower(strings.TrimSpace(request.Slug))
	if name == "" || slug == "" {
		return tenancy.Organization{}, errors.New("organization name and slug are required")
	}
	organization := tenancy.Organization{ID: uuid.NewString(), Name: name, Slug: slug}
	err := s.db.QueryRow(ctx, `INSERT INTO organizations(id,name,slug) VALUES($1,$2,$3) RETURNING created_at,updated_at`, organization.ID, organization.Name, organization.Slug).Scan(&organization.CreatedAt, &organization.UpdatedAt)
	return organization, err
}

func (s *Store) CreateProject(ctx context.Context, request CreateProjectRequest) (tenancy.Project, error) {
	project := tenancy.Project{ID: uuid.NewString(), OrganizationID: strings.TrimSpace(request.OrganizationID), Name: strings.TrimSpace(request.Name), Slug: strings.ToLower(strings.TrimSpace(request.Slug))}
	if project.OrganizationID == "" || project.Name == "" || project.Slug == "" {
		return tenancy.Project{}, errors.New("organization id, project name, and slug are required")
	}
	err := s.db.QueryRow(ctx, `INSERT INTO projects(id,organization_id,name,slug) VALUES($1,$2::uuid,$3,$4) RETURNING created_at,updated_at`, project.ID, project.OrganizationID, project.Name, project.Slug).Scan(&project.CreatedAt, &project.UpdatedAt)
	return project, err
}

func (s *Store) CreateEnvironment(ctx context.Context, request CreateEnvironmentRequest) (tenancy.Environment, error) {
	environment := tenancy.Environment{ID: uuid.NewString(), ProjectID: strings.TrimSpace(request.ProjectID), Name: strings.TrimSpace(request.Name), Slug: strings.ToLower(strings.TrimSpace(request.Slug)), Production: request.Production}
	if environment.ProjectID == "" || environment.Name == "" || environment.Slug == "" {
		return tenancy.Environment{}, errors.New("project id, environment name, and slug are required")
	}
	err := s.db.QueryRow(ctx, `INSERT INTO environments(id,project_id,name,slug,production) VALUES($1,$2::uuid,$3,$4,$5) RETURNING created_at,updated_at`, environment.ID, environment.ProjectID, environment.Name, environment.Slug, environment.Production).Scan(&environment.CreatedAt, &environment.UpdatedAt)
	return environment, err
}

func (s *Store) CreateWorkload(ctx context.Context, request CreateWorkloadRequest) (platformworkloads.Workload, platformworkloads.Revision, error) {
	if err := request.Kind.Validate(); err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	if strings.TrimSpace(request.EnvironmentID) == "" || strings.TrimSpace(request.Name) == "" {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, errors.New("environment id and workload name are required")
	}
	if len(request.Spec) == 0 {
		request.Spec = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Spec) {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, errors.New("workload spec must be valid JSON")
	}
	if request.DesiredState == "" {
		request.DesiredState = platformworkloads.DesiredState("stopped")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	defer tx.Rollback(ctx)
	workload := platformworkloads.Workload{ID: uuid.NewString(), EnvironmentID: strings.TrimSpace(request.EnvironmentID), Kind: request.Kind, Name: strings.TrimSpace(request.Name), DesiredGeneration: 1, DesiredState: request.DesiredState, ObservedState: platformworkloads.ObservedState("unknown")}
	if err := tx.QueryRow(ctx, `INSERT INTO workloads(id,environment_id,kind,name,desired_generation,desired_state,observed_state) VALUES($1,$2::uuid,$3,$4,$5,$6,$7) RETURNING created_at,updated_at`, workload.ID, workload.EnvironmentID, workload.Kind, workload.Name, workload.DesiredGeneration, workload.DesiredState, workload.ObservedState).Scan(&workload.CreatedAt, &workload.UpdatedAt); err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	revision := platformworkloads.Revision{ID: uuid.NewString(), WorkloadID: workload.ID, Number: 1, SchemaVersion: 1, Spec: append(json.RawMessage(nil), request.Spec...)}
	if err := tx.QueryRow(ctx, `INSERT INTO workload_revisions(id,workload_id,number,schema_version,spec,created_by) VALUES($1,$2::uuid,$3,$4,$5::jsonb,NULLIF($6,'')::uuid) RETURNING created_at`, revision.ID, revision.WorkloadID, revision.Number, revision.SchemaVersion, []byte(revision.Spec), strings.TrimSpace(request.CreatedBy)).Scan(&revision.CreatedAt); err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE workloads SET current_revision_id=$2::uuid,updated_at=now() WHERE id=$1::uuid`, workload.ID, revision.ID); err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	workload.CurrentRevisionID = revision.ID
	if err := tx.Commit(ctx); err != nil {
		return platformworkloads.Workload{}, platformworkloads.Revision{}, err
	}
	return workload, revision, nil
}

func (s *Store) ListWorkloads(ctx context.Context, environmentID string) ([]platformworkloads.Workload, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text,environment_id::text,kind,name,desired_generation,observed_generation,desired_state,observed_state,COALESCE(current_revision_id::text,''),last_observation_at,last_reconcile_error,created_at,updated_at FROM workloads WHERE ($1='' OR environment_id=$1::uuid) ORDER BY created_at DESC`, strings.TrimSpace(environmentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []platformworkloads.Workload{}
	for rows.Next() {
		var value platformworkloads.Workload
		if err := rows.Scan(&value.ID, &value.EnvironmentID, &value.Kind, &value.Name, &value.DesiredGeneration, &value.ObservedGeneration, &value.DesiredState, &value.ObservedState, &value.CurrentRevisionID, &value.LastObservationAt, &value.LastReconcileError, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) RecordWorkloadObservation(ctx context.Context, workloadID string, generation int64, state platformworkloads.ObservedState, details json.RawMessage) error {
	if generation < 0 || strings.TrimSpace(string(state)) == "" {
		return errors.New("observation generation and state are required")
	}
	if len(details) == 0 {
		details = json.RawMessage(`{}`)
	}
	if !json.Valid(details) {
		return errors.New("observation details must be valid JSON")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var desired int64
	if err := tx.QueryRow(ctx, `SELECT desired_generation FROM workloads WHERE id=$1::uuid FOR UPDATE`, workloadID).Scan(&desired); err != nil {
		return fmt.Errorf("load workload: %w", err)
	}
	if generation > desired {
		return errors.New("observation generation exceeds desired generation")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workload_observations(id,workload_id,generation,state,details) VALUES($1,$2::uuid,$3,$4,$5::jsonb)`, uuid.NewString(), workloadID, generation, state, []byte(details)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workloads SET observed_generation=GREATEST(observed_generation,$2),observed_state=$3,last_observation_at=now(),last_reconcile_error='',updated_at=now() WHERE id=$1::uuid`, workloadID, generation, state)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateAgentCommand(ctx context.Context, request CreateAgentCommandRequest) (AgentCommand, error) {
	request.NodeID, request.CommandType, request.IdempotencyKey = strings.TrimSpace(request.NodeID), strings.TrimSpace(request.CommandType), strings.TrimSpace(request.IdempotencyKey)
	if request.NodeID == "" || request.CommandType == "" || request.IdempotencyKey == "" {
		return AgentCommand{}, errors.New("node id, command type, and idempotency key are required")
	}
	if request.ProtocolVersion < 1 {
		request.ProtocolVersion = 1
	}
	if len(request.Payload) == 0 {
		request.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Payload) {
		return AgentCommand{}, errors.New("command payload must be valid JSON")
	}
	value := AgentCommand{}
	err := s.db.QueryRow(ctx, `INSERT INTO agent_commands(id,node_id,workload_id,operation_id,command_type,protocol_version,idempotency_key,payload) VALUES($1,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8::jsonb) ON CONFLICT(node_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key RETURNING id::text,node_id::text,COALESCE(workload_id::text,''),COALESCE(operation_id::text,''),command_type,protocol_version,idempotency_key,payload,status,acknowledgement,error,issued_at,acknowledged_at,completed_at`, uuid.NewString(), request.NodeID, strings.TrimSpace(request.WorkloadID), strings.TrimSpace(request.OperationID), request.CommandType, request.ProtocolVersion, request.IdempotencyKey, []byte(request.Payload)).Scan(&value.ID, &value.NodeID, &value.WorkloadID, &value.OperationID, &value.CommandType, &value.ProtocolVersion, &value.IdempotencyKey, &value.Payload, &value.Status, &value.Acknowledgement, &value.Error, &value.IssuedAt, &value.AcknowledgedAt, &value.CompletedAt)
	return value, err
}

func (s *Store) PendingAgentCommands(ctx context.Context, nodeID string, limit int) ([]AgentCommand, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id::text,node_id::text,COALESCE(workload_id::text,''),COALESCE(operation_id::text,''),command_type,protocol_version,idempotency_key,payload,status,acknowledgement,error,issued_at,acknowledged_at,completed_at FROM agent_commands WHERE node_id=$1::uuid AND status IN ('queued','delivered') ORDER BY issued_at LIMIT $2`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []AgentCommand{}
	for rows.Next() {
		var value AgentCommand
		if err := rows.Scan(&value.ID, &value.NodeID, &value.WorkloadID, &value.OperationID, &value.CommandType, &value.ProtocolVersion, &value.IdempotencyKey, &value.Payload, &value.Status, &value.Acknowledgement, &value.Error, &value.IssuedAt, &value.AcknowledgedAt, &value.CompletedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) AcknowledgeAgentCommand(ctx context.Context, id, status, message string, acknowledgement json.RawMessage) error {
	return s.acknowledgeAgentCommand(ctx, id, "", status, message, acknowledgement)
}

// AcknowledgeAgentCommandForNode prevents one Beacon from completing another
// node's command when using the node-authenticated remote API.
func (s *Store) AcknowledgeAgentCommandForNode(ctx context.Context, id, nodeID, status, message string, acknowledgement json.RawMessage) error {
	if strings.TrimSpace(nodeID) == "" {
		return errors.New("node id is required")
	}
	return s.acknowledgeAgentCommand(ctx, id, nodeID, status, message, acknowledgement)
}

func (s *Store) acknowledgeAgentCommand(ctx context.Context, id, nodeID, status, message string, acknowledgement json.RawMessage) error {
	status = strings.TrimSpace(status)
	switch status {
	case "acknowledged", "succeeded", "failed", "cancelled":
	default:
		return errors.New("invalid agent command acknowledgement status")
	}
	if len(acknowledgement) == 0 {
		acknowledgement = json.RawMessage(`{}`)
	}
	if !json.Valid(acknowledgement) {
		return errors.New("acknowledgement must be valid JSON")
	}
	completed := status == "succeeded" || status == "failed" || status == "cancelled"
	command, err := s.db.Exec(ctx, `UPDATE agent_commands SET status=$2,acknowledgement=$3::jsonb,error=$4,acknowledged_at=COALESCE(acknowledged_at,now()),completed_at=CASE WHEN $5 THEN now() ELSE completed_at END WHERE id=$1::uuid AND ($6='' OR node_id=$6::uuid)`, id, status, []byte(acknowledgement), strings.TrimSpace(message), completed, strings.TrimSpace(nodeID))
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("agent command not found")
	}
	return nil
}
