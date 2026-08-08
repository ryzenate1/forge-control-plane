package policies

import (
	"context"
	"gamepanel/forge/internal/store"
)

// ProjectPolicy enforces organization tenancy for projects. It replaces the
// previous permissive implementation that returned true universally. All checks
// resolve the project's org via GetProject and then verify team_members
// membership via UserIsOrgMember / GetTeamMemberRole.
type ProjectPolicy struct {
	st *store.Store
}

func NewProjectPolicy(st *store.Store) *ProjectPolicy {
	return &ProjectPolicy{st: st}
}

func (p *ProjectPolicy) Name() string { return "project" }

func (p *ProjectPolicy) Can(ctx context.Context, user store.User, action Action, resource any) bool {
	if user.Role == "admin" {
		return true
	}
	if p.st == nil {
		return false
	}
	projectID, ok := resourceAsID(resource)
	if !ok {
		return false
	}
	project, err := p.st.GetProject(ctx, projectID)
	if err != nil {
		return false
	}
	// Any team member may read; mutations require owner/admin.
	isMember, err := p.st.UserIsOrgMember(ctx, project.OrgID, user.ID)
	if err != nil || !isMember {
		return false
	}
	if action == ActionRead {
		return true
	}
	// For create/update/delete require elevated org role.
	role, err := p.st.GetTeamMemberRole(ctx, project.OrgID, user.ID)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

func resourceAsID(resource any) (string, bool) {
	switch v := resource.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case store.Project:
		if v.ID == "" {
			return "", false
		}
		return v.ID, true
	case *store.Project:
		if v == nil || v.ID == "" {
			return "", false
		}
		return v.ID, true
	default:
		return "", false
	}
}
