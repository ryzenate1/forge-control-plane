package tenancy

import (
	"context"
	"errors"
	"strings"

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
