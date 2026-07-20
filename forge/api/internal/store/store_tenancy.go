package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	OwnerID   string    `json:"ownerId"`
	OwnerName string    `json:"ownerName,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Project struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"orgId"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Environment struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Protected bool      `json:"protected"`
	CreatedAt time.Time `json:"createdAt"`
}

type TeamMember struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Email     string    `json:"email,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateOrganizationRequest struct {
	Name string
	Slug string
}

type CreateProjectRequest struct {
	Name        string
	Slug        string
	Description string
}

type CreateEnvironmentRequest struct {
	Name      string
	Color     string
	Protected bool
}

var validTeamRoles = map[string]bool{
	"owner":  true,
	"admin":  true,
	"member": true,
	"viewer": true,
}

var allowedEnvColors = map[string]bool{
	"#6366f1": true, "#22c55e": true, "#f59e0b": true,
	"#ef4444": true, "#8b5cf6": true, "#06b6d4": true,
	"#ec4899": true, "#64748b": true,
}

func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	if slug == "" {
		slug = uuid.NewString()[:8]
	}
	return slug
}

// ---- Organizations ----

func (s *Store) ListOrganizationsForUser(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id::text, o.name, o.slug, o.owner_id::text, COALESCE(u.email, ''), o.created_at
		FROM organizations o
		JOIN team_members tm ON tm.org_id = o.id AND tm.user_id = $1
		JOIN users u ON u.id = o.owner_id
		ORDER BY o.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orgs := []Organization{}
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerID, &o.OwnerName, &o.CreatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (s *Store) GetOrganizationBySlug(ctx context.Context, slug string) (Organization, error) {
	var o Organization
	err := s.db.QueryRow(ctx, `
		SELECT o.id::text, o.name, o.slug, o.owner_id::text, COALESCE(u.email, ''), o.created_at
		FROM organizations o
		JOIN users u ON u.id = o.owner_id
		WHERE o.slug = $1
	`, slug).Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerID, &o.OwnerName, &o.CreatedAt)
	if err != nil {
		return Organization{}, err
	}
	return o, nil
}

func (s *Store) CreateOrganization(ctx context.Context, req CreateOrganizationRequest, ownerID string, actorID *string) (Organization, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Organization{}, errors.New("name is required")
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}
	slug = strings.ToLower(slug)

	orgID := uuid.NewString()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_id)
		VALUES ($1, $2, $3, $4)
	`, orgID, name, slug, ownerID); err != nil {
		return Organization{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (id, org_id, user_id, role)
		VALUES ($1, $2, $3, 'owner')
	`, uuid.NewString(), orgID, ownerID); err != nil {
		return Organization{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Organization{}, err
	}

	_ = s.AppendAudit(ctx, actorID, "organization created", "organization", &orgID, `{"name":"`+name+`","slug":"`+slug+`"}`)

	var o Organization
	err = s.db.QueryRow(ctx, `
		SELECT id::text, name, slug, owner_id::text, created_at
		FROM organizations WHERE id = $1
	`, orgID).Scan(&o.ID, &o.Name, &o.Slug, &o.OwnerID, &o.CreatedAt)
	return o, err
}

func (s *Store) DeleteOrganization(ctx context.Context, orgID string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("organization not found")
	}
	_ = s.AppendAudit(ctx, actorID, "organization deleted", "organization", &orgID, `{}`)
	return nil
}

func (s *Store) OrganizationOwnedByUser(ctx context.Context, orgID, userID string) (bool, error) {
	var ownerID string
	err := s.db.QueryRow(ctx, `SELECT owner_id::text FROM organizations WHERE id = $1`, orgID).Scan(&ownerID)
	if err != nil {
		return false, err
	}
	return ownerID == userID, nil
}

// ---- Projects ----

func (s *Store) ListProjectsByOrg(ctx context.Context, orgID string) ([]Project, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, org_id::text, name, slug, COALESCE(description, ''), created_at
		FROM projects
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, projectID string) (Project, error) {
	var p Project
	err := s.db.QueryRow(ctx, `
		SELECT id::text, org_id::text, name, slug, COALESCE(description, ''), created_at
		FROM projects WHERE id = $1
	`, projectID).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt)
	if err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Store) CreateProject(ctx context.Context, orgID string, req CreateProjectRequest, actorID *string) (Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Project{}, errors.New("name is required")
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}
	slug = strings.ToLower(slug)

	projectID := uuid.NewString()
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO projects (id, org_id, name, slug, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, projectID, orgID, name, slug, strings.TrimSpace(req.Description), now); err != nil {
		return Project{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "project created", "project", &projectID, `{"name":"`+name+`","orgId":"`+orgID+`"}`)
	return Project{ID: projectID, OrgID: orgID, Name: name, Slug: slug, Description: strings.TrimSpace(req.Description), CreatedAt: now}, nil
}

func (s *Store) UpdateProject(ctx context.Context, projectID string, req CreateProjectRequest) (Project, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Project{}, errors.New("name is required")
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}
	slug = strings.ToLower(slug)

	if _, err := s.db.Exec(ctx, `
		UPDATE projects SET name = $1, slug = $2, description = $3
		WHERE id = $4
	`, name, slug, strings.TrimSpace(req.Description), projectID); err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, projectID)
}

func (s *Store) DeleteProject(ctx context.Context, projectID string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("project not found")
	}
	_ = s.AppendAudit(ctx, actorID, "project deleted", "project", &projectID, `{}`)
	return nil
}

// ---- Environments ----

func (s *Store) ListEnvironmentsByProject(ctx context.Context, projectID string) ([]Environment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, project_id::text, name, color, COALESCE(protected, false), created_at
		FROM environments
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	envs := []Environment{}
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Color, &e.Protected, &e.CreatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

func (s *Store) CreateEnvironment(ctx context.Context, projectID string, req CreateEnvironmentRequest, actorID *string) (Environment, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Environment{}, errors.New("name is required")
	}
	color := strings.TrimSpace(req.Color)
	if color == "" || !allowedEnvColors[color] {
		color = "#6366f1"
	}

	envID := uuid.NewString()
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO environments (id, project_id, name, color, protected, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, envID, projectID, name, color, req.Protected, now); err != nil {
		return Environment{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "environment created", "environment", &envID, `{"name":"`+name+`","projectId":"`+projectID+`"}`)
	return Environment{ID: envID, ProjectID: projectID, Name: name, Color: color, Protected: req.Protected, CreatedAt: now}, nil
}

func (s *Store) UpdateEnvironment(ctx context.Context, envID string, req CreateEnvironmentRequest) (Environment, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Environment{}, errors.New("name is required")
	}
	color := strings.TrimSpace(req.Color)
	if color == "" || !allowedEnvColors[color] {
		color = "#6366f1"
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE environments SET name = $1, color = $2, protected = $3
		WHERE id = $4
	`, name, color, req.Protected, envID); err != nil {
		return Environment{}, err
	}
	var e Environment
	err := s.db.QueryRow(ctx, `
		SELECT id::text, project_id::text, name, color, COALESCE(protected, false), created_at
		FROM environments WHERE id = $1
	`, envID).Scan(&e.ID, &e.ProjectID, &e.Name, &e.Color, &e.Protected, &e.CreatedAt)
	return e, err
}

func (s *Store) DeleteEnvironment(ctx context.Context, envID string, actorID *string) error {
	// Prevent deleting protected environments
	var protected bool
	var name string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(protected, false), name FROM environments WHERE id = $1`, envID).Scan(&protected, &name); err != nil {
		return err
	}
	if protected {
		return errors.New("cannot delete protected environment")
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM environments WHERE id = $1`, envID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("environment not found")
	}
	_ = s.AppendAudit(ctx, actorID, "environment deleted", "environment", &envID, `{"name":"`+name+`"}`)
	return nil
}

// ---- Team Members ----

func (s *Store) ListTeamMembers(ctx context.Context, orgID string) ([]TeamMember, error) {
	rows, err := s.db.Query(ctx, `
		SELECT tm.id::text, tm.org_id::text, tm.user_id::text, COALESCE(u.email, ''), tm.role, tm.created_at
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.org_id = $1
		ORDER BY
			CASE tm.role
				WHEN 'owner' THEN 1
				WHEN 'admin' THEN 2
				WHEN 'member' THEN 3
				WHEN 'viewer' THEN 4
				ELSE 5
			END,
			u.email
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []TeamMember{}
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Email, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *Store) GetTeamMemberRole(ctx context.Context, orgID, userID string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx, `
		SELECT role FROM team_members WHERE org_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return role, nil
}

func (s *Store) AddTeamMember(ctx context.Context, orgID, userID, role, actorID string) (TeamMember, error) {
	if !validTeamRoles[role] {
		return TeamMember{}, errors.New("invalid role: must be owner, admin, member, or viewer")
	}

	// Verify user exists
	var email string
	if err := s.db.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 AND NOT disabled`, userID).Scan(&email); err != nil {
		return TeamMember{}, errors.New("user not found")
	}

	memberID := uuid.NewString()
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO team_members (id, org_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, memberID, orgID, userID, role, now); err != nil {
		return TeamMember{}, err
	}
	var actorIDPtr *string
	if actorID != "" {
		actorIDPtr = &actorID
	}
	_ = s.AppendAudit(ctx, actorIDPtr, "team member added", "organization", &orgID, `{"userId":"`+userID+`","role":"`+role+`"}`)
	return TeamMember{ID: memberID, OrgID: orgID, UserID: userID, Email: email, Role: role, CreatedAt: now}, nil
}

func (s *Store) UpdateTeamMemberRole(ctx context.Context, orgID, userID, role, actorID string) error {
	if !validTeamRoles[role] {
		return errors.New("invalid role: must be owner, admin, member, or viewer")
	}
	// Prevent demoting the last owner
	if role != "owner" {
		var ownerCount int
		if err := s.db.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE org_id = $1 AND role = 'owner'`, orgID).Scan(&ownerCount); err != nil {
			return err
		}
		if ownerCount <= 1 {
			var currentRole string
			if err := s.db.QueryRow(ctx, `SELECT role FROM team_members WHERE org_id = $1 AND user_id = $2`, orgID, userID).Scan(&currentRole); err == nil && currentRole == "owner" {
				return errors.New("cannot remove the last owner")
			}
		}
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE team_members SET role = $1 WHERE org_id = $2 AND user_id = $3
	`, role, orgID, userID)
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
	_ = s.AppendAudit(ctx, actorIDPtr, "team member role updated", "organization", &orgID, `{"userId":"`+userID+`","role":"`+role+`"}`)
	return nil
}

func (s *Store) RemoveTeamMember(ctx context.Context, orgID, userID, actorID string) error {
	// Prevent removing the last owner
	var currentRole string
	if err := s.db.QueryRow(ctx, `SELECT role FROM team_members WHERE org_id = $1 AND user_id = $2`, orgID, userID).Scan(&currentRole); err != nil {
		return errors.New("member not found")
	}
	if currentRole == "owner" {
		var ownerCount int
		if err := s.db.QueryRow(ctx, `SELECT count(*) FROM team_members WHERE org_id = $1 AND role = 'owner'`, orgID).Scan(&ownerCount); err != nil {
			return err
		}
		if ownerCount <= 1 {
			return errors.New("cannot remove the last owner")
		}
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM team_members WHERE org_id = $1 AND user_id = $2`, orgID, userID)
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
	_ = s.AppendAudit(ctx, actorIDPtr, "team member removed", "organization", &orgID, `{"userId":"`+userID+`"}`)
	return nil
}

func (s *Store) UserIsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_members WHERE org_id = $1 AND user_id = $2)`, orgID, userID).Scan(&exists)
	return exists, err
}

func (s *Store) ResolveEffectiveOrgRole(ctx context.Context, orgID, userID, globalRole string) string {
	if globalRole == "admin" {
		return "admin"
	}
	role, err := s.GetTeamMemberRole(ctx, orgID, userID)
	if err != nil || role == "" {
		return ""
	}
	return role
}

// ---- Scoped Resource Queries ----

func (s *Store) ListServersForOrg(ctx context.Context, orgID string, page, perPage int, search string) ([]Server, int, error) {
	offset := (page - 1) * perPage

	countQuery := `SELECT count(*) FROM servers WHERE org_id = $1`
	baseQuery := `
		SELECT s.id::text, s.name, COALESCE(s.description, ''), s.status, s.desired_state::text, s.actual_state::text, s.config_sync_pending, s.suspended, s.transferring, s.transfer_target_node_id::text, s.transfer_state, s.transfer_error, s.transfer_run_token::text, s.memory_mb, s.cpu_shares, s.disk_mb, n.name, u.email, e.name
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN users u ON u.id = s.owner_id
		JOIN eggs e ON e.id = s.egg_id
		WHERE s.org_id = $1`

	args := []any{orgID}
	argIdx := 2

	if search != "" {
		countQuery += ` AND (s.name ILIKE $2 OR s.description ILIKE $3)`
		baseQuery += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.description ILIKE $%d)", argIdx, argIdx+1)
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern)
		argIdx += 2
	}

	baseQuery += fmt.Sprintf(" ORDER BY s.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, perPage, offset)

	var total int
	countArgs := args[:len(args)-2]
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	servers := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState, &server.ConfigSyncPending, &server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState, &server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template); err != nil {
			return nil, 0, err
		}
		servers = append(servers, server)
	}
	return servers, total, rows.Err()
}

func (s *Store) CountServersForOrg(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM servers WHERE org_id = $1`, orgID).Scan(&count)
	return count, err
}
