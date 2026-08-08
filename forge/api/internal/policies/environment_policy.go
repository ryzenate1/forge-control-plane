package policies

import (
	"context"
	"gamepanel/forge/internal/store"
)

// EnvironmentPolicy enforces org tenancy for environments. Environments are
// nested under projects, so the policy resolves the environment's project via
// DB lookup, then the project's org via GetProject, and finally checks
// team_members membership via UserIsOrgMember. This replaces the prior
// universal-true implementation.
type EnvironmentPolicy struct {
	st *store.Store
}

func NewEnvironmentPolicy(st *store.Store) *EnvironmentPolicy {
	return &EnvironmentPolicy{st: st}
}

func (p *EnvironmentPolicy) Name() string { return "environment" }

func (p *EnvironmentPolicy) Can(ctx context.Context, user store.User, action Action, resource any) bool {
	if user.Role == "admin" {
		return true
	}
	if p.st == nil {
		return false
	}
	envID, ok := resourceAsEnvID(resource)
	if !ok {
		return false
	}
	// Resolve project_id for the environment.
	var projectID string
	if err := p.st.DB().QueryRow(ctx, `SELECT project_id::text FROM environments WHERE id = $1`, envID).Scan(&projectID); err != nil || projectID == "" {
		return false
	}
	project, err := p.st.GetProject(ctx, projectID)
	if err != nil {
		return false
	}
	isMember, err := p.st.UserIsOrgMember(ctx, project.OrgID, user.ID)
	if err != nil || !isMember {
		return false
	}
	if action == ActionRead {
		return true
	}
	role, err := p.st.GetTeamMemberRole(ctx, project.OrgID, user.ID)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

func resourceAsEnvID(resource any) (string, bool) {
	switch v := resource.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case store.Environment:
		if v.ID == "" {
			return "", false
		}
		return v.ID, true
	case *store.Environment:
		if v == nil || v.ID == "" {
			return "", false
		}
		return v.ID, true
	default:
		return "", false
	}
}
