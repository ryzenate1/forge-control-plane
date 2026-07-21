package tenancy

import (
	"context"
	"errors"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

type Service struct {
	store *store.Store
}

func New(st *store.Store) *Service {
	return &Service{store: st}
}

type CreateOrganizationInput struct {
	Name   string
	Slug   string
	UserID string
	Actor  *string
}

type CreateProjectInput struct {
	Name        string
	Slug        string
	Description string
	Actor       *string
}

type CreateEnvironmentInput struct {
	Name      string
	Color     string
	Protected bool
	Actor     *string
}

type AddMemberInput struct {
	OrgID   string
	UserID  string
	Role    string
	ActorID string
}

type UpdateMemberInput struct {
	OrgID   string
	UserID  string
	Role    string
	ActorID string
}

type RemoveMemberInput struct {
	OrgID   string
	UserID  string
	ActorID string
}

type OrgContext struct {
	OrgID string
	Role  string
}

type orgContextKey struct{}

func SetOrgContext(ctx context.Context, oc OrgContext) context.Context {
	return context.WithValue(ctx, orgContextKey{}, oc)
}

func GetOrgContext(ctx context.Context) (OrgContext, bool) {
	oc, ok := ctx.Value(orgContextKey{}).(OrgContext)
	return oc, ok
}

func (svc *Service) CreateOrganization(ctx context.Context, input CreateOrganizationInput) (store.Organization, error) {
	if input.UserID == "" {
		return store.Organization{}, errors.New("user id is required")
	}
	req := store.CreateOrganizationRequest{
		Name: input.Name,
		Slug: input.Slug,
	}
	return svc.store.CreateOrganization(ctx, req, input.UserID, input.Actor)
}

func (svc *Service) ListOrganizations(ctx context.Context, userID string) ([]store.Organization, error) {
	return svc.store.ListOrganizationsForUser(ctx, userID)
}

func (svc *Service) GetOrganization(ctx context.Context, slug string) (store.Organization, error) {
	return svc.store.GetOrganizationBySlug(ctx, slug)
}

func (svc *Service) DeleteOrganization(ctx context.Context, orgID string, actorID *string) error {
	return svc.store.DeleteOrganization(ctx, orgID, actorID)
}

func (svc *Service) CreateProject(ctx context.Context, orgID string, input CreateProjectInput) (store.Project, error) {
	req := store.CreateProjectRequest{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
	}
	return svc.store.CreateProject(ctx, orgID, req, input.Actor)
}

func (svc *Service) ListProjects(ctx context.Context, orgID string) ([]store.Project, error) {
	return svc.store.ListProjectsByOrg(ctx, orgID)
}

func (svc *Service) GetProject(ctx context.Context, projectID string) (store.Project, error) {
	return svc.store.GetProject(ctx, projectID)
}

func (svc *Service) UpdateProject(ctx context.Context, projectID string, input CreateProjectInput) (store.Project, error) {
	req := store.CreateProjectRequest{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
	}
	return svc.store.UpdateProject(ctx, projectID, req)
}

func (svc *Service) DeleteProject(ctx context.Context, projectID string, actorID *string) error {
	return svc.store.DeleteProject(ctx, projectID, actorID)
}

func (svc *Service) CreateEnvironment(ctx context.Context, projectID string, input CreateEnvironmentInput) (store.Environment, error) {
	req := store.CreateEnvironmentRequest{
		Name:      input.Name,
		Color:     input.Color,
		Protected: input.Protected,
	}
	return svc.store.CreateEnvironment(ctx, projectID, req, input.Actor)
}

func (svc *Service) ListEnvironments(ctx context.Context, projectID string) ([]store.Environment, error) {
	return svc.store.ListEnvironmentsByProject(ctx, projectID)
}

func (svc *Service) UpdateEnvironment(ctx context.Context, envID string, input CreateEnvironmentInput) (store.Environment, error) {
	req := store.CreateEnvironmentRequest{
		Name:      input.Name,
		Color:     input.Color,
		Protected: input.Protected,
	}
	return svc.store.UpdateEnvironment(ctx, envID, req)
}

func (svc *Service) DeleteEnvironment(ctx context.Context, envID string, actorID *string) error {
	return svc.store.DeleteEnvironment(ctx, envID, actorID)
}

func (svc *Service) AddTeamMember(ctx context.Context, input AddMemberInput) (store.TeamMember, error) {
	role := strings.ToLower(strings.TrimSpace(input.Role))
	if role == "" {
		role = "member"
	}
	return svc.store.AddTeamMember(ctx, input.OrgID, input.UserID, role, input.ActorID)
}

func (svc *Service) ListTeamMembers(ctx context.Context, orgID string) ([]store.TeamMember, error) {
	return svc.store.ListTeamMembers(ctx, orgID)
}

func (svc *Service) UpdateMemberRole(ctx context.Context, input UpdateMemberInput) error {
	return svc.store.UpdateTeamMemberRole(ctx, input.OrgID, input.UserID, input.Role, input.ActorID)
}

func (svc *Service) RemoveTeamMember(ctx context.Context, input RemoveMemberInput) error {
	return svc.store.RemoveTeamMember(ctx, input.OrgID, input.UserID, input.ActorID)
}

func (svc *Service) ResolvePermissions(ctx context.Context, orgID, userID, globalRole string) string {
	return svc.store.ResolveEffectiveOrgRole(ctx, orgID, userID, globalRole)
}

func (svc *Service) UserIsOrgMember(ctx context.Context, orgID, userID string) (bool, error) {
	return svc.store.UserIsOrgMember(ctx, orgID, userID)
}

func (svc *Service) ScopeServersByOrg(ctx context.Context, orgID string, page, perPage int, search string) ([]store.Server, int, error) {
	return svc.store.ListServersForOrg(ctx, orgID, page, perPage, search)
}

func (svc *Service) AssignServerToOrg(ctx context.Context, serverID, orgID string) error {
	_, err := svc.store.DB().Exec(ctx, `UPDATE servers SET org_id = $1 WHERE id = $2`, orgID, serverID)
	return err
}

func (svc *Service) AssignServerToProject(ctx context.Context, serverID, projectID string) error {
	_, err := svc.store.DB().Exec(ctx, `UPDATE servers SET project_id = $1 WHERE id = $2`, projectID, serverID)
	return err
}

func (svc *Service) AssignServerToEnvironment(ctx context.Context, serverID, envID string) error {
	_, err := svc.store.DB().Exec(ctx, `UPDATE servers SET environment_id = $1 WHERE id = $2`, envID, serverID)
	return err
}

// ---- Authorization Helpers ----

// CheckServerAccess returns an error if the user cannot access the given
// server. Admins always pass. Non-admins must own the server or be an
// org member whose org owns the server.
func (svc *Service) CheckServerAccess(ctx context.Context, serverID, userID, role string) error {
	if role == "admin" {
		return nil
	}

	// First check direct ownership or subuser access
	allowed, err := svc.store.UserCanAccessServer(ctx, serverID, userID, role, "")
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}

	// Fall back to org membership check
	return errors.New("server access denied")
}

// RequireOrgRole returns an error when the user's effective role in the org
// is below the minimum. Admins always pass. valid roles: owner, admin,
// member, viewer.
func (svc *Service) RequireOrgRole(ctx context.Context, orgID, userID, globalRole, minRole string) error {
	if globalRole == "admin" {
		return nil
	}
	effective := svc.store.ResolveEffectiveOrgRole(ctx, orgID, userID, globalRole)
	if effective == "" {
		return errors.New("not a member of this organization")
	}
	rank := map[string]int{"owner": 4, "admin": 3, "member": 2, "viewer": 1}
	if rank[effective] < rank[minRole] {
		return errors.New("insufficient organization role")
	}
	return nil
}

// CanManageOrg returns an error when the user is not an owner or admin of
// the org. Admins always pass.
func (svc *Service) CanManageOrg(ctx context.Context, orgID, userID, globalRole string) error {
	if globalRole == "admin" {
		return nil
	}
	role := svc.store.ResolveEffectiveOrgRole(ctx, orgID, userID, globalRole)
	if role != "owner" && role != "admin" {
		return errors.New("insufficient organization permissions")
	}
	return nil
}

// ---- Invitations ----

type CreateInvitationInput struct {
	OrgID     string
	Email     string
	Role      string
	InvitedBy string
	TTL       time.Duration
}

func (svc *Service) CreateInvitation(ctx context.Context, input CreateInvitationInput) (store.OrgInvitation, error) {
	return svc.store.CreateInvitation(ctx, store.CreateInvitationRequest{
		OrgID:     input.OrgID,
		Email:     input.Email,
		Role:      input.Role,
		InvitedBy: input.InvitedBy,
		TTL:       input.TTL,
	})
}

func (svc *Service) ListInvitations(ctx context.Context, orgID string) ([]store.OrgInvitation, error) {
	return svc.store.ListInvitations(ctx, orgID)
}

func (svc *Service) AcceptInvitation(ctx context.Context, token, userID string) error {
	return svc.store.AcceptInvitation(ctx, token, userID)
}

func (svc *Service) RevokeInvitation(ctx context.Context, orgID, invitationID string) error {
	return svc.store.RevokeInvitation(ctx, orgID, invitationID)
}

// ---- Granular Permissions ----

func (svc *Service) GetMemberPermissions(ctx context.Context, orgID, userID string) (store.GranularPermissions, error) {
	return svc.store.GetTeamMemberPermissions(ctx, orgID, userID)
}

func (svc *Service) SetMemberPermissions(ctx context.Context, orgID, userID string, perms store.GranularPermissions, actorID string) error {
	return svc.store.SetTeamMemberPermissions(ctx, orgID, userID, perms, actorID)
}

func (svc *Service) HasPermission(ctx context.Context, orgID, userID, globalRole, permission string) (bool, error) {
	if globalRole == "admin" {
		return true, nil
	}
	role := svc.store.ResolveEffectiveOrgRole(ctx, orgID, userID, globalRole)
	if role == "owner" || role == "admin" {
		return true, nil
	}
	perms, err := svc.store.GetTeamMemberPermissions(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	switch permission {
	case "canCreateProjects":
		return perms.CanCreateProjects, nil
	case "canDeleteProjects":
		return perms.CanDeleteProjects, nil
	case "canCreateEnvironments":
		return perms.CanCreateEnvironments, nil
	case "canDeleteEnvironments":
		return perms.CanDeleteEnvironments, nil
	case "canCreateServices":
		return perms.CanCreateServices, nil
	case "canDeleteServices":
		return perms.CanDeleteServices, nil
	case "canManageMembers":
		return perms.CanManageMembers, nil
	case "canManageEnvVars":
		return perms.CanManageEnvVars, nil
	case "canManageBackups":
		return perms.CanManageBackups, nil
	case "canViewSensitiveEnv":
		return perms.CanViewSensitiveEnv, nil
	}
	return false, nil
}
