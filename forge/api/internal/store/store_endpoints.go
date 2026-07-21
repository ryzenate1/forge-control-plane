package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"

	"github.com/google/uuid"
)

type InfraEndpoint struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	EndpointType      string            `json:"endpointType"`
	ConnectionMode    string            `json:"connectionMode"`
	Status            string            `json:"status"`
	EdgeID            string            `json:"edgeId,omitempty"`
	Tags              []string          `json:"tags"`
	Labels            []domain.LabelPair `json:"labels"`
	URL               string            `json:"url,omitempty"`
	ProjectID         string            `json:"projectId,omitempty"`
	GroupID           string            `json:"groupId,omitempty"`
	Reachable         bool              `json:"reachable"`
	Version           string            `json:"version,omitempty"`
	TotalContainers   int               `json:"totalContainers"`
	TotalImages       int               `json:"totalImages"`
	TotalVolumes      int               `json:"totalVolumes"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

type InfraEndpointNode struct {
	ID         string    `json:"id"`
	EndpointID string    `json:"endpointId"`
	NodeID     string    `json:"nodeId"`
	CreatedAt  time.Time `json:"createdAt"`
}

type InfraEndpointAccessPolicy struct {
	ID            string    `json:"id"`
	EndpointID    string    `json:"endpointId"`
	PrincipalType string    `json:"principalType"`
	PrincipalID   string    `json:"principalId"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"createdAt"`
}

type InfraEndpointHealthHistory struct {
	ID             string    `json:"id"`
	EndpointID     string    `json:"endpointId"`
	Status         string    `json:"status"`
	Reachable      bool      `json:"reachable"`
	HealthScore    float64   `json:"healthScore"`
	Version        string    `json:"version,omitempty"`
	TotalContainers int      `json:"totalContainers"`
	TotalImages    int       `json:"totalImages"`
	TotalVolumes   int       `json:"totalVolumes"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	ObservedAt     time.Time `json:"observedAt"`
}

var validEndpointTypes = map[string]bool{
	"docker":     true,
	"swarm":      true,
	"kubernetes": true,
	"edge":       true,
}

var validConnectionModes = map[string]bool{
	"direct": true,
	"tunnel": true,
	"edge":   true,
}

func (s *Store) ListInfraEndpoints(ctx context.Context) ([]InfraEndpoint, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text, e.name, COALESCE(e.description, ''), e.endpoint_type, e.connection_mode,
		       e.status, COALESCE(e.edge_id, ''), COALESCE(e.tags, '{}'), COALESCE(e.labels, '[]'),
		       COALESCE(e.url, ''), COALESCE(e.project_id, ''), COALESCE(e.group_id, ''),
		       COALESCE(e.reachable, false), COALESCE(e.version, ''),
		       COALESCE(e.total_container_count, 0), COALESCE(e.total_image_count, 0),
		       COALESCE(e.total_volume_count, 0), e.created_at, e.updated_at,
		       (SELECT count(*) FROM infra_endpoint_nodes WHERE endpoint_id = e.id) as node_count
		FROM infra_endpoints e
		ORDER BY e.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	endpoints := []InfraEndpoint{}
	for rows.Next() {
		var ep InfraEndpoint
		if err := rows.Scan(
			&ep.ID, &ep.Name, &ep.Description, &ep.EndpointType, &ep.ConnectionMode,
			&ep.Status, &ep.EdgeID, &ep.Tags, &ep.Labels,
			&ep.URL, &ep.ProjectID, &ep.GroupID,
			&ep.Reachable, &ep.Version,
			&ep.TotalContainers, &ep.TotalImages, &ep.TotalVolumes,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

func (s *Store) GetInfraEndpoint(ctx context.Context, id string) (InfraEndpoint, error) {
	var ep InfraEndpoint
	err := s.db.QueryRow(ctx, `
		SELECT e.id::text, e.name, COALESCE(e.description, ''), e.endpoint_type, e.connection_mode,
		       e.status, COALESCE(e.edge_id, ''), COALESCE(e.tags, '{}'), COALESCE(e.labels, '[]'),
		       COALESCE(e.url, ''), COALESCE(e.project_id, ''), COALESCE(e.group_id, ''),
		       COALESCE(e.reachable, false), COALESCE(e.version, ''),
		       COALESCE(e.total_container_count, 0), COALESCE(e.total_image_count, 0),
		       COALESCE(e.total_volume_count, 0), e.created_at, e.updated_at
		FROM infra_endpoints e
		WHERE e.id = $1
	`, id).Scan(
		&ep.ID, &ep.Name, &ep.Description, &ep.EndpointType, &ep.ConnectionMode,
		&ep.Status, &ep.EdgeID, &ep.Tags, &ep.Labels,
		&ep.URL, &ep.ProjectID, &ep.GroupID,
		&ep.Reachable, &ep.Version,
		&ep.TotalContainers, &ep.TotalImages, &ep.TotalVolumes,
		&ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		return InfraEndpoint{}, err
	}
	return ep, nil
}

func (s *Store) CreateInfraEndpoint(ctx context.Context, req domain.CreateEndpointRequest, actorID *string) (InfraEndpoint, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return InfraEndpoint{}, errors.New("name is required")
	}
	epType := strings.ToLower(strings.TrimSpace(string(req.EndpointType)))
	if epType == "" {
		epType = "docker"
	}
	if !validEndpointTypes[epType] {
		return InfraEndpoint{}, errors.New("invalid endpoint type: must be docker, swarm, kubernetes, or edge")
	}
	connMode := strings.ToLower(strings.TrimSpace(string(req.ConnectionMode)))
	if connMode == "" {
		connMode = "direct"
	}
	if !validConnectionModes[connMode] {
		return InfraEndpoint{}, errors.New("invalid connection mode: must be direct, tunnel, or edge")
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	labelsJSON := "[]"
	if len(req.Labels) > 0 {
		var jsonErr error
		labelsJSON, jsonErr = marshalJSON(req.Labels)
		if jsonErr != nil {
			return InfraEndpoint{}, jsonErr
		}
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO infra_endpoints (id, name, description, endpoint_type, connection_mode, status, edge_id, tags, labels, url, project_id, group_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, id, name, strings.TrimSpace(req.Description), epType, connMode,
		domain.EndpointStatusUnknown, strings.TrimSpace(req.EdgeID), req.Tags, labelsJSON,
		strings.TrimSpace(req.URL), strings.TrimSpace(req.ProjectID), strings.TrimSpace(req.GroupID),
		now, now)
	if err != nil {
		return InfraEndpoint{}, err
	}

	for _, nodeID := range req.NodeIDs {
		nid := strings.TrimSpace(nodeID)
		if nid == "" {
			continue
		}
		memberID := uuid.NewString()
		if _, err := s.db.Exec(ctx, `
			INSERT INTO infra_endpoint_nodes (id, endpoint_id, node_id, created_at)
			VALUES ($1, $2, $3, $4)
		`, memberID, id, nid, now); err != nil {
			_, _ = s.db.Exec(ctx, `DELETE FROM infra_endpoints WHERE id = $1`, id)
			return InfraEndpoint{}, err
		}
	}

	_ = s.AppendAudit(ctx, actorID, "infra endpoint created", "infra_endpoint", &id, `{"name":"`+name+`"}`)
	return s.GetInfraEndpoint(ctx, id)
}

func (s *Store) UpdateInfraEndpoint(ctx context.Context, id string, req domain.UpdateEndpointRequest, actorID *string) (InfraEndpoint, error) {
	current, err := s.GetInfraEndpoint(ctx, id)
	if err != nil {
		return InfraEndpoint{}, err
	}

	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	desc := current.Description
	if req.Description != nil {
		desc = strings.TrimSpace(*req.Description)
	}
	epType := current.EndpointType
	if req.EndpointType != nil {
		et := strings.ToLower(strings.TrimSpace(string(*req.EndpointType)))
		if !validEndpointTypes[et] {
			return InfraEndpoint{}, errors.New("invalid endpoint type")
		}
		epType = et
	}
	connMode := current.ConnectionMode
	if req.ConnectionMode != nil {
		cm := strings.ToLower(strings.TrimSpace(string(*req.ConnectionMode)))
		if !validConnectionModes[cm] {
			return InfraEndpoint{}, errors.New("invalid connection mode")
		}
		connMode = cm
	}
	tags := current.Tags
	if req.Tags != nil {
		tags = *req.Tags
	}
	labels := current.Labels
	if req.Labels != nil {
		labels = *req.Labels
	}
	url := current.URL
	if req.URL != nil {
		url = strings.TrimSpace(*req.URL)
	}
	projectID := current.ProjectID
	if req.ProjectID != nil {
		projectID = strings.TrimSpace(*req.ProjectID)
	}
	groupID := current.GroupID
	if req.GroupID != nil {
		groupID = strings.TrimSpace(*req.GroupID)
	}

	labelsJSON, jsonErr := marshalJSON(labels)
	if jsonErr != nil {
		return InfraEndpoint{}, jsonErr
	}

	_, err = s.db.Exec(ctx, `
		UPDATE infra_endpoints
		SET name = $1, description = $2, endpoint_type = $3, connection_mode = $4,
		    tags = $5, labels = $6, url = $7, project_id = $8, group_id = $9, updated_at = now()
		WHERE id = $10
	`, name, desc, epType, connMode, tags, labelsJSON, url, projectID, groupID, id)
	if err != nil {
		return InfraEndpoint{}, err
	}

	_ = s.AppendAudit(ctx, actorID, "infra endpoint updated", "infra_endpoint", &id, `{}`)
	return s.GetInfraEndpoint(ctx, id)
}

func (s *Store) DeleteInfraEndpoint(ctx context.Context, id string, actorID *string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM infra_endpoints WHERE id = $1`, id)
	if err != nil {
		return err
	}
	_ = s.AppendAudit(ctx, actorID, "infra endpoint deleted", "infra_endpoint", &id, `{}`)
	return nil
}

func (s *Store) ListInfraEndpointNodes(ctx context.Context, endpointID string) ([]InfraEndpointNode, error) {
	rows, err := s.db.Query(ctx, `
		SELECT en.id::text, en.endpoint_id::text, en.node_id::text, en.created_at
		FROM infra_endpoint_nodes en
		WHERE en.endpoint_id = $1
		ORDER BY en.created_at
	`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []InfraEndpointNode{}
	for rows.Next() {
		var n InfraEndpointNode
		if err := rows.Scan(&n.ID, &n.EndpointID, &n.NodeID, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) AddNodeToInfraEndpoint(ctx context.Context, endpointID, nodeID string) error {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO infra_endpoint_nodes (id, endpoint_id, node_id, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint_id, node_id) DO NOTHING
	`, id, endpointID, nodeID, now)
	return err
}

func (s *Store) RemoveNodeFromInfraEndpoint(ctx context.Context, endpointID, nodeID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM infra_endpoint_nodes WHERE endpoint_id = $1 AND node_id = $2`, endpointID, nodeID)
	return err
}

func (s *Store) ListInfraEndpointAccessPolicies(ctx context.Context, endpointID string) ([]InfraEndpointAccessPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, endpoint_id::text, principal_type, principal_id, role, created_at
		FROM infra_endpoint_access_policies
		WHERE endpoint_id = $1
		ORDER BY created_at
	`, endpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []InfraEndpointAccessPolicy{}
	for rows.Next() {
		var p InfraEndpointAccessPolicy
		if err := rows.Scan(&p.ID, &p.EndpointID, &p.PrincipalType, &p.PrincipalID, &p.Role, &p.CreatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) SetInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID, role string) (InfraEndpointAccessPolicy, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO infra_endpoint_access_policies (id, endpoint_id, principal_type, principal_id, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint_id, principal_type, principal_id)
		DO UPDATE SET role = EXCLUDED.role
	`, id, endpointID, principalType, principalID, role, now)
	if err != nil {
		return InfraEndpointAccessPolicy{}, err
	}
	var p InfraEndpointAccessPolicy
	err = s.db.QueryRow(ctx, `
		SELECT id::text, endpoint_id::text, principal_type, principal_id, role, created_at
		FROM infra_endpoint_access_policies
		WHERE endpoint_id = $1 AND principal_type = $2 AND principal_id = $3
	`, endpointID, principalType, principalID).Scan(&p.ID, &p.EndpointID, &p.PrincipalType, &p.PrincipalID, &p.Role, &p.CreatedAt)
	return p, err
}

func (s *Store) RemoveInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM infra_endpoint_access_policies WHERE endpoint_id = $1 AND principal_type = $2 AND principal_id = $3`, endpointID, principalType, principalID)
	return err
}

func (s *Store) RecordEndpointHealth(ctx context.Context, endpointID string, status string, reachable bool, score float64, version string, containers, images, volumes int, errMsg string) error {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, e := s.db.Exec(ctx, `
		INSERT INTO infra_endpoint_health_history (id, endpoint_id, status, reachable, health_score, version, total_containers, total_images, total_volumes, error_message, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, endpointID, status, reachable, score, version, containers, images, volumes, errMsg, now)
	return e
}

func (s *Store) ListEndpointHealthHistory(ctx context.Context, endpointID string, limit int) ([]InfraEndpointHealthHistory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, endpoint_id::text, status, reachable, COALESCE(health_score, 0),
		       COALESCE(version, ''), COALESCE(total_containers, 0), COALESCE(total_images, 0),
		       COALESCE(total_volumes, 0), COALESCE(error_message, ''), observed_at
		FROM infra_endpoint_health_history
		WHERE endpoint_id = $1
		ORDER BY observed_at DESC
		LIMIT $2
	`, endpointID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []InfraEndpointHealthHistory{}
	for rows.Next() {
		var r InfraEndpointHealthHistory
		if err := rows.Scan(&r.ID, &r.EndpointID, &r.Status, &r.Reachable, &r.HealthScore,
			&r.Version, &r.TotalContainers, &r.TotalImages, &r.TotalVolumes,
			&r.ErrorMessage, &r.ObservedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) EndpointCanAcceptNode(ctx context.Context, endpointID, nodeID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM infra_endpoint_nodes WHERE endpoint_id = $1 AND node_id = $2)
	`, endpointID, nodeID).Scan(&exists)
	return !exists, err
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
