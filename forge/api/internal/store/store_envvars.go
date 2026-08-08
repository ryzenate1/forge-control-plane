package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type EnvironmentVariable struct {
	ID             string    `json:"id"`
	OrgID          *string   `json:"orgId,omitempty"`
	ProjectID      *string   `json:"projectId,omitempty"`
	EnvironmentID  *string   `json:"environmentId,omitempty"`
	ServiceID      *string   `json:"serviceId,omitempty"`
	Scope          string    `json:"scope"`
	Key            string    `json:"key"`
	ValueEncrypted string    `json:"-"`
	IsSensitive    bool      `json:"isSensitive"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type EnvVarRevision struct {
	ID             string    `json:"id"`
	VariableID     string    `json:"variableId"`
	Version        int       `json:"version"`
	ValueEncrypted string    `json:"-"`
	CreatedBy      *string   `json:"createdBy,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateEnvVarRequest struct {
	OrgID         *string
	ProjectID     *string
	EnvironmentID *string
	ServiceID     *string
	Scope         string
	Key           string
	Value         string
	IsSensitive   bool
}

type UpdateEnvVarRequest struct {
	Value       string
	IsSensitive bool
}

type OrgInvitation struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"orgId"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Token      string     `json:"token"`
	InvitedBy  *string    `json:"invitedBy,omitempty"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type CreateInvitationRequest struct {
	OrgID     string
	Email     string
	Role      string
	InvitedBy string
	TTL       time.Duration
}

type GranularPermissions struct {
	CanCreateProjects     bool `json:"canCreateProjects"`
	CanDeleteProjects     bool `json:"canDeleteProjects"`
	CanCreateEnvironments bool `json:"canCreateEnvironments"`
	CanDeleteEnvironments bool `json:"canDeleteEnvironments"`
	CanCreateServices     bool `json:"canCreateServices"`
	CanDeleteServices     bool `json:"canDeleteServices"`
	CanManageMembers      bool `json:"canManageMembers"`
	CanManageEnvVars      bool `json:"canManageEnvVars"`
	CanManageBackups      bool `json:"canManageBackups"`
	CanViewSensitiveEnv   bool `json:"canViewSensitiveEnv"`
}

func DefaultGranularPermissions(role string) GranularPermissions {
	switch role {
	case "owner":
		return GranularPermissions{true, true, true, true, true, true, true, true, true, true}
	case "admin":
		return GranularPermissions{true, true, true, true, true, true, true, true, true, true}
	case "member":
		return GranularPermissions{true, false, true, false, true, false, false, true, true, false}
	default:
		return GranularPermissions{}
	}
}

func normalizeScope(scope string) string {
	s := strings.ToLower(strings.TrimSpace(scope))
	switch s {
	case "organization", "project", "environment", "service":
		return s
	default:
		return "environment"
	}
}

// ---- Environment Variables ----

func (s *Store) CreateEnvironmentVariable(ctx context.Context, req CreateEnvVarRequest, actorID *string) (EnvironmentVariable, error) {
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return EnvironmentVariable{}, errors.New("key is required")
	}
	scope := normalizeScope(req.Scope)

	var encrypted string
	if req.Value != "" {
		var err error
		encrypted, err = s.encryptSecret(req.Value, secretAAD("environment_variables", uuid.NewString(), key))
		if err != nil && !errors.Is(err, ErrSecretEncryptionUnavailable) {
			return EnvironmentVariable{}, err
		}
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	if _, err := s.db.Exec(ctx, `
		INSERT INTO environment_variables (id, org_id, project_id, environment_id, service_id, scope, key, value_encrypted, is_sensitive, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1, $10, $10)
	`, id, req.OrgID, req.ProjectID, req.EnvironmentID, req.ServiceID, scope, key, encrypted, req.IsSensitive, now); err != nil {
		if pgErr := isUniqueViolation(err); pgErr != nil {
			return EnvironmentVariable{}, fmt.Errorf("variable with key %q already exists in this scope", key)
		}
		return EnvironmentVariable{}, err
	}

	_ = s.AppendAudit(ctx, actorID, "env_var created", "environment_variable", &id, fmt.Sprintf(`{"key":"%s","scope":"%s"}`, key, scope))
	return EnvironmentVariable{ID: id, OrgID: req.OrgID, ProjectID: req.ProjectID, EnvironmentID: req.EnvironmentID, ServiceID: req.ServiceID, Scope: scope, Key: key, ValueEncrypted: encrypted, IsSensitive: req.IsSensitive, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetEnvironmentVariable(ctx context.Context, id string) (EnvironmentVariable, error) {
	var v EnvironmentVariable
	err := s.db.QueryRow(ctx, `
		SELECT id::text, org_id::text, project_id::text, environment_id::text, service_id::text, scope, key, value_encrypted, is_sensitive, version, created_at, updated_at
		FROM environment_variables WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&v.ID, &v.OrgID, &v.ProjectID, &v.EnvironmentID, &v.ServiceID, &v.Scope, &v.Key, &v.ValueEncrypted, &v.IsSensitive, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (s *Store) ListEnvironmentVariables(ctx context.Context, scopeType, scopeID string) ([]EnvironmentVariable, error) {
	scope := normalizeScope(scopeType)
	col := scope + "_id"
	query := fmt.Sprintf(`
		SELECT id::text, org_id::text, project_id::text, environment_id::text, service_id::text, scope, key, value_encrypted, is_sensitive, version, created_at, updated_at
		FROM environment_variables
		WHERE %s = $1 AND scope = $2 AND deleted_at IS NULL
		ORDER BY key ASC
	`, col)

	rows, err := s.db.Query(ctx, query, scopeID, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vars := []EnvironmentVariable{}
	for rows.Next() {
		var v EnvironmentVariable
		if err := rows.Scan(&v.ID, &v.OrgID, &v.ProjectID, &v.EnvironmentID, &v.ServiceID, &v.Scope, &v.Key, &v.ValueEncrypted, &v.IsSensitive, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func (s *Store) UpdateEnvironmentVariable(ctx context.Context, id string, req UpdateEnvVarRequest, actorID *string) (EnvironmentVariable, error) {
	existing, err := s.GetEnvironmentVariable(ctx, id)
	if err != nil {
		return EnvironmentVariable{}, err
	}

	now := time.Now().UTC()
	version := existing.Version + 1

	var encrypted string
	if req.Value != "" {
		var encErr error
		encrypted, encErr = s.encryptSecret(req.Value, secretAAD("environment_variables", id, existing.Key))
		if encErr != nil && !errors.Is(encErr, ErrSecretEncryptionUnavailable) {
			return EnvironmentVariable{}, encErr
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EnvironmentVariable{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO environment_variable_revisions (id, variable_id, version, value_encrypted, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.NewString(), id, existing.Version, existing.ValueEncrypted, actorID, now); err != nil {
		return EnvironmentVariable{}, err
	}

	upsert := `UPDATE environment_variables SET value_encrypted = $1, is_sensitive = $2, version = $3, updated_at = $4 WHERE id = $5 AND deleted_at IS NULL`
	if _, err := tx.Exec(ctx, upsert, encrypted, req.IsSensitive, version, now, id); err != nil {
		return EnvironmentVariable{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return EnvironmentVariable{}, err
	}

	_ = s.AppendAudit(ctx, actorID, "env_var updated", "environment_variable", &id, fmt.Sprintf(`{"key":"%s","version":%d}`, existing.Key, version))
	return s.GetEnvironmentVariable(ctx, id)
}

func (s *Store) DeleteEnvironmentVariable(ctx context.Context, id string, actorID *string) error {
	now := time.Now().UTC()
	tag, err := s.db.Exec(ctx, `UPDATE environment_variables SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("environment variable not found")
	}
	_ = s.AppendAudit(ctx, actorID, "env_var deleted", "environment_variable", &id, `{}`)
	return nil
}

func (s *Store) ResolveEnvironmentVariables(ctx context.Context, orgID, projectID, envID, serviceID string) (map[string]string, error) {
	result := map[string]string{}

	resolveOrder := []struct {
		col   string
		scope string
		id    string
	}{
		{"org_id", "organization", orgID},
		{"project_id", "project", projectID},
		{"environment_id", "environment", envID},
		{"service_id", "service", serviceID},
	}

	for _, r := range resolveOrder {
		if r.id == "" {
			continue
		}
		rows, err := s.db.Query(ctx, fmt.Sprintf(`
			SELECT key, value_encrypted
			FROM environment_variables
			WHERE %s = $1 AND scope = $2 AND deleted_at IS NULL
			ORDER BY key ASC
		`, r.col), r.id, r.scope)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key, encrypted string
			if err := rows.Scan(&key, &encrypted); err != nil {
				rows.Close()
				return nil, err
			}
			if encrypted != "" {
				plain, decErr := s.decryptSecret(encrypted, "", secretAAD("environment_variables", fmt.Sprintf("%s:%s", r.scope, r.id), key))
				if decErr == nil {
					result[key] = plain
				}
			}
		}
		rows.Close()
	}

	return result, nil
}

func (s *Store) GetEnvVarRevisions(ctx context.Context, variableID string) ([]EnvVarRevision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, variable_id::text, version, value_encrypted, created_by::text, created_at
		FROM environment_variable_revisions
		WHERE variable_id = $1
		ORDER BY version DESC
	`, variableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := []EnvVarRevision{}
	for rows.Next() {
		var r EnvVarRevision
		if err := rows.Scan(&r.ID, &r.VariableID, &r.Version, &r.ValueEncrypted, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		revisions = append(revisions, r)
	}
	return revisions, rows.Err()
}

// ---- Organization Invitations ----

func (s *Store) CreateInvitation(ctx context.Context, req CreateInvitationRequest) (OrgInvitation, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return OrgInvitation{}, errors.New("email is required")
	}
	role := strings.TrimSpace(req.Role)
	if !validTeamRoles[role] {
		return OrgInvitation{}, errors.New("invalid role")
	}
	if req.TTL <= 0 {
		req.TTL = 7 * 24 * time.Hour
	}

	id := uuid.NewString()
	token := uuid.NewString()
	now := time.Now().UTC()

	if _, err := s.db.Exec(ctx, `
		INSERT INTO org_invitations (id, org_id, email, role, token, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, req.OrgID, email, role, token, req.InvitedBy, now.Add(req.TTL), now); err != nil {
		return OrgInvitation{}, err
	}

	return OrgInvitation{ID: id, OrgID: req.OrgID, Email: email, Role: role, Token: token, InvitedBy: &req.InvitedBy, ExpiresAt: now.Add(req.TTL), CreatedAt: now}, nil
}

func (s *Store) ListInvitations(ctx context.Context, orgID string) ([]OrgInvitation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, org_id::text, email, role, token, invited_by::text, accepted_at, expires_at, revoked_at, created_at
		FROM org_invitations
		WHERE org_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := []OrgInvitation{}
	for rows.Next() {
		var inv OrgInvitation
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.Token, &inv.InvitedBy, &inv.AcceptedAt, &inv.ExpiresAt, &inv.RevokedAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

func (s *Store) AcceptInvitation(ctx context.Context, token, userID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var inv OrgInvitation
	err = tx.QueryRow(ctx, `
		SELECT id::text, org_id::text, email, role, token, invited_by::text, accepted_at, expires_at, revoked_at, created_at
		FROM org_invitations WHERE token = $1 FOR UPDATE
	`, token).Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.Token, &inv.InvitedBy, &inv.AcceptedAt, &inv.ExpiresAt, &inv.RevokedAt, &inv.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("invitation not found")
		}
		return err
	}
	if inv.AcceptedAt != nil {
		return errors.New("invitation already accepted")
	}
	if inv.RevokedAt != nil {
		return errors.New("invitation has been revoked")
	}
	if time.Now().After(inv.ExpiresAt) {
		return errors.New("invitation has expired")
	}

	var userEmail string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail); err != nil {
		return errors.New("user not found")
	}

	if _, err := tx.Exec(ctx, `UPDATE org_invitations SET accepted_at = now() WHERE id = $1`, inv.ID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (id, org_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, uuid.NewString(), inv.OrgID, userID, inv.Role); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) RevokeInvitation(ctx context.Context, orgID, invitationID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE org_invitations SET revoked_at = now() WHERE id = $1 AND org_id = $2 AND accepted_at IS NULL`, invitationID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("invitation not found or already accepted")
	}
	return nil
}

func (s *Store) GetTeamMemberPermissions(ctx context.Context, orgID, userID string) (GranularPermissions, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT COALESCE(permissions, '{}'::jsonb) FROM team_members WHERE org_id = $1 AND user_id = $2`, orgID, userID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DefaultGranularPermissions("viewer"), nil
		}
		return GranularPermissions{}, err
	}

	var perms GranularPermissions
	if len(raw) > 0 && string(raw) != "{}" {
		_ = json.Unmarshal(raw, &perms)
	}

	role, _ := s.GetTeamMemberRole(ctx, orgID, userID)
	defaultPerms := DefaultGranularPermissions(role)

	if !perms.CanCreateProjects && role != "" {
		perms.CanCreateProjects = defaultPerms.CanCreateProjects
	}
	if !perms.CanDeleteProjects {
		perms.CanDeleteProjects = defaultPerms.CanDeleteProjects
	}
	if !perms.CanCreateEnvironments {
		perms.CanCreateEnvironments = defaultPerms.CanCreateEnvironments
	}
	if !perms.CanDeleteEnvironments {
		perms.CanDeleteEnvironments = defaultPerms.CanDeleteEnvironments
	}
	if !perms.CanCreateServices {
		perms.CanCreateServices = defaultPerms.CanCreateServices
	}
	if !perms.CanDeleteServices {
		perms.CanDeleteServices = defaultPerms.CanDeleteServices
	}
	if !perms.CanManageMembers {
		perms.CanManageMembers = defaultPerms.CanManageMembers
	}
	if !perms.CanManageEnvVars {
		perms.CanManageEnvVars = defaultPerms.CanManageEnvVars
	}
	if !perms.CanManageBackups {
		perms.CanManageBackups = defaultPerms.CanManageBackups
	}
	if !perms.CanViewSensitiveEnv {
		perms.CanViewSensitiveEnv = defaultPerms.CanViewSensitiveEnv
	}

	return perms, nil
}

func (s *Store) SetTeamMemberPermissions(ctx context.Context, orgID, userID string, perms GranularPermissions, actorID string) error {
	raw, err := json.Marshal(perms)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE team_members SET permissions = $1::jsonb WHERE org_id = $2 AND user_id = $3`, string(raw), orgID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("member not found")
	}
	var actorIDPtr *string
	if actorID != "" {
		actorIDPtr = &actorID
	}
	_ = s.AppendAudit(ctx, actorIDPtr, "team member permissions updated", "organization", &orgID, `{"userId":"`+userID+`","permissions":"`+string(raw)+`"}`)
	return nil
}

func isUniqueViolation(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "unique") || strings.Contains(errStr, "23505") {
		return fmt.Errorf("duplicate value violates unique constraint")
	}
	return nil
}
