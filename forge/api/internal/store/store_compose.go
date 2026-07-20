package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ComposeStack struct {
	ID            string            `json:"id"`
	UserID        string            `json:"userId"`
	Name          string            `json:"name"`
	NodeID        string            `json:"nodeId"`
	Status        string            `json:"status"`
	ComposeYAML   string            `json:"composeYaml"`
	ComposeHash   string            `json:"composeHash"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	MemoryMB      int64             `json:"memoryMb"`
	CPUShares     int64             `json:"cpuShares"`
	DiskMB        int64             `json:"diskMb"`
	Error         string            `json:"error,omitempty"`
	ReservationID string            `json:"reservationId,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

func (s *Store) CreateComposeStack(ctx context.Context, stack *ComposeStack) error {
	if s.db == nil {
		return nil
	}
	if stack.CreatedAt.IsZero() {
		stack.CreatedAt = time.Now().UTC()
	}
	if stack.UpdatedAt.IsZero() {
		stack.UpdatedAt = stack.CreatedAt
	}
	envJSON, err := marshalEnvVars(stack.EnvVars)
	if err != nil {
		return fmt.Errorf("marshal env vars: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO compose_stacks (
			id, user_id, name, node_id, status, compose_yaml, compose_hash,
			env_vars, memory_mb, cpu_shares, disk_mb, error, reservation_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			compose_yaml = EXCLUDED.compose_yaml,
			compose_hash = EXCLUDED.compose_hash,
			env_vars = EXCLUDED.env_vars,
			memory_mb = EXCLUDED.memory_mb,
			cpu_shares = EXCLUDED.cpu_shares,
			disk_mb = EXCLUDED.disk_mb,
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
	`, stack.ID, stack.UserID, stack.Name, stack.NodeID, stack.Status,
		stack.ComposeYAML, stack.ComposeHash, envJSON, stack.MemoryMB, stack.CPUShares,
		stack.DiskMB, stack.Error, nullIfEmpty(stack.ReservationID),
		stack.CreatedAt, stack.UpdatedAt)
	return err
}

func (s *Store) UpdateComposeStack(ctx context.Context, stack *ComposeStack) error {
	return s.CreateComposeStack(ctx, stack)
}

func (s *Store) GetComposeStack(ctx context.Context, stackID string) (ComposeStack, error) {
	if s.db == nil {
		return ComposeStack{}, nil
	}
	var stack ComposeStack
	var reservationID, errorMsg *string
	var envJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, node_id, status, compose_yaml, compose_hash,
		       env_vars, memory_mb, cpu_shares, disk_mb, COALESCE(error, ''), reservation_id,
		       created_at, updated_at
		FROM compose_stacks
		WHERE id = $1
	`, stackID).Scan(
		&stack.ID, &stack.UserID, &stack.Name, &stack.NodeID, &stack.Status,
		&stack.ComposeYAML, &stack.ComposeHash, &envJSON, &stack.MemoryMB, &stack.CPUShares,
		&stack.DiskMB, &errorMsg, &reservationID, &stack.CreatedAt, &stack.UpdatedAt,
	)
	if err != nil {
		return ComposeStack{}, err
	}
	if errorMsg != nil {
		stack.Error = *errorMsg
	}
	if reservationID != nil {
		stack.ReservationID = *reservationID
	}
	if len(envJSON) > 0 {
		_ = json.Unmarshal(envJSON, &stack.EnvVars)
	}
	if stack.EnvVars == nil {
		stack.EnvVars = make(map[string]string)
	}
	return stack, nil
}

func (s *Store) ListComposeStacks(ctx context.Context, userID string) ([]ComposeStack, error) {
	if s.db == nil {
		return nil, nil
	}
	query := `SELECT id, user_id, name, node_id, status, compose_yaml, compose_hash,
		       env_vars, memory_mb, cpu_shares, disk_mb, COALESCE(error, ''), reservation_id,
		       created_at, updated_at
		FROM compose_stacks`
	args := []any{}

	if userID != "" {
		query += ` WHERE user_id = $1`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stacks := []ComposeStack{}
	for rows.Next() {
		var stack ComposeStack
		var reservationID, errorMsg *string
		var envJSON []byte
		if err := rows.Scan(
			&stack.ID, &stack.UserID, &stack.Name, &stack.NodeID, &stack.Status,
			&stack.ComposeYAML, &stack.ComposeHash, &envJSON, &stack.MemoryMB, &stack.CPUShares,
			&stack.DiskMB, &errorMsg, &reservationID, &stack.CreatedAt, &stack.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if errorMsg != nil {
			stack.Error = *errorMsg
		}
		if reservationID != nil {
			stack.ReservationID = *reservationID
		}
		if len(envJSON) > 0 {
			_ = json.Unmarshal(envJSON, &stack.EnvVars)
		}
		if stack.EnvVars == nil {
			stack.EnvVars = make(map[string]string)
		}
		stacks = append(stacks, stack)
	}
	return stacks, rows.Err()
}

func (s *Store) DeleteComposeStack(ctx context.Context, stackID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM compose_stacks WHERE id = $1`, stackID)
	return err
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func marshalEnvVars(envVars map[string]string) ([]byte, error) {
	if envVars == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(envVars)
}

type ProjectDocument struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ServerID       string `json:"serverId,omitempty"`
	ComposeContent string `json:"composeContent"`
	ParsedConfig   []byte `json:"parsedConfig"`
	Status         string `json:"status"`
	Revision       int    `json:"revision"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func (s *Store) CreateComposeProject(ctx context.Context, p *ProjectDocument) error {
	if p.ID == "" {
		return fmt.Errorf("project ID is required")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_projects (id, name, server_id, compose_content, parsed_config, status, revision, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, NOW(), NOW())
	`, p.ID, p.Name, p.ServerID, p.ComposeContent, p.ParsedConfig, p.Status, p.Revision)
	return err
}

func (s *Store) GetComposeProject(ctx context.Context, id string) (*ProjectDocument, error) {
	var p ProjectDocument
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(server_id::text, ''), compose_content, parsed_config::text, status, revision,
		       TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
		FROM compose_projects
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.ServerID, &p.ComposeContent, &p.ParsedConfig, &p.Status, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListComposeProjects(ctx context.Context) ([]ProjectDocument, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(server_id::text, ''), compose_content, COALESCE(parsed_config::text, '{}'), status, revision,
		       TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at,
		       TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
		FROM compose_projects
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []ProjectDocument
	for rows.Next() {
		var p ProjectDocument
		if err := rows.Scan(&p.ID, &p.Name, &p.ServerID, &p.ComposeContent, &p.ParsedConfig, &p.Status, &p.Revision, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateComposeProject(ctx context.Context, p *ProjectDocument) error {
	_, err := s.db.Exec(ctx, `
		UPDATE compose_projects
		SET name = $2, compose_content = $3, parsed_config = $4, status = $5, revision = $6, updated_at = NOW()
		WHERE id = $1
	`, p.ID, p.Name, p.ComposeContent, p.ParsedConfig, p.Status, p.Revision)
	return err
}

func (s *Store) DeleteComposeProject(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM compose_projects WHERE id = $1`, id)
	return err
}
