package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationCRUD(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "orgtest@example.com",
		Password: "TestPassword123!",
		Role:     "user",
	}, nil)
	require.NoError(t, err)

	org, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Test Org",
		Slug: "test-org",
	}, user.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, "Test Org", org.Name)
	assert.Equal(t, "test-org", org.Slug)
	assert.Equal(t, user.ID, org.OwnerID)

	orgBySlug, err := store.GetOrganizationBySlug(ctx, "test-org")
	require.NoError(t, err)
	assert.Equal(t, org.ID, orgBySlug.ID)

	orgs, err := store.ListOrganizationsForUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, orgs, 1)
	assert.Equal(t, "Test Org", orgs[0].Name)

	err = store.DeleteOrganization(ctx, org.ID, nil)
	require.NoError(t, err)
}

func TestCrossOrgAccessRejection(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner1, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "owner1@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	owner2, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "owner2@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	org1, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Org One", Slug: "org-one",
	}, owner1.ID, nil)
	require.NoError(t, err)

	org2, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Org Two", Slug: "org-two",
	}, owner2.ID, nil)
	require.NoError(t, err)

	isMember, err := store.UserIsOrgMember(ctx, org1.ID, owner2.ID)
	require.NoError(t, err)
	assert.False(t, isMember, "owner2 should not be a member of org1")

	isMember, err = store.UserIsOrgMember(ctx, org2.ID, owner1.ID)
	require.NoError(t, err)
	assert.False(t, isMember, "owner1 should not be a member of org2")

	role := store.ResolveEffectiveOrgRole(ctx, org1.ID, owner1.ID, "user")
	assert.Equal(t, "owner", role)

	role = store.ResolveEffectiveOrgRole(ctx, org1.ID, owner2.ID, "user")
	assert.Empty(t, role)

	_ = org1
	_ = org2
}

func TestTeamMemberCRUD(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "teamowner@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	member, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "teammember@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	org, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Team Org", Slug: "team-org",
	}, owner.ID, nil)
	require.NoError(t, err)

	tm, err := store.AddTeamMember(ctx, org.ID, member.ID, "member", owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "member", tm.Role)
	assert.Equal(t, member.ID, tm.UserID)

	members, err := store.ListTeamMembers(ctx, org.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)

	err = store.UpdateTeamMemberRole(ctx, org.ID, member.ID, "admin", owner.ID)
	require.NoError(t, err)

	role := store.ResolveEffectiveOrgRole(ctx, org.ID, member.ID, "user")
	assert.Equal(t, "admin", role)

	err = store.RemoveTeamMember(ctx, org.ID, member.ID, owner.ID)
	require.NoError(t, err)

	members, err = store.ListTeamMembers(ctx, org.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
}

func TestRoleBasedPermissions(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "rbacowner@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	admin, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "rbacadmin@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	viewer, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "rbacviewer@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	org, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "RBAC Org", Slug: "rbac-org",
	}, owner.ID, nil)
	require.NoError(t, err)

	_, err = store.AddTeamMember(ctx, org.ID, admin.ID, "admin", owner.ID)
	require.NoError(t, err)
	_, err = store.AddTeamMember(ctx, org.ID, viewer.ID, "viewer", owner.ID)
	require.NoError(t, err)

	assert.Equal(t, "owner", store.ResolveEffectiveOrgRole(ctx, org.ID, owner.ID, "user"))
	assert.Equal(t, "admin", store.ResolveEffectiveOrgRole(ctx, org.ID, admin.ID, "user"))
	assert.Equal(t, "viewer", store.ResolveEffectiveOrgRole(ctx, org.ID, viewer.ID, "user"))

	isMember, _ := store.UserIsOrgMember(ctx, org.ID, owner.ID)
	assert.True(t, isMember)
	isMember, _ = store.UserIsOrgMember(ctx, org.ID, admin.ID)
	assert.True(t, isMember)
	isMember, _ = store.UserIsOrgMember(ctx, org.ID, viewer.ID)
	assert.True(t, isMember)
}

func TestProjectCRUD(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "projectowner@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	org, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Project Org", Slug: "project-org",
	}, owner.ID, nil)
	require.NoError(t, err)

	project, err := store.CreateProject(ctx, org.ID, CreateProjectRequest{
		Name: "My Project", Slug: "my-project", Description: "Test description",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "My Project", project.Name)
	assert.Equal(t, "my-project", project.Slug)

	projects, err := store.ListProjectsByOrg(ctx, org.ID)
	require.NoError(t, err)
	assert.Len(t, projects, 1)

	updated, err := store.UpdateProject(ctx, project.ID, CreateProjectRequest{
		Name: "Renamed Project", Slug: "renamed-project", Description: "Updated",
	})
	require.NoError(t, err)
	assert.Equal(t, "Renamed Project", updated.Name)

	err = store.DeleteProject(ctx, project.ID, nil)
	require.NoError(t, err)
}

func TestEnvironmentCRUD(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner, err := store.CreateUser(ctx, CreateUserRequest{
		Email:    "envowner@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	org, err := store.CreateOrganization(ctx, CreateOrganizationRequest{
		Name: "Env Org", Slug: "env-org",
	}, owner.ID, nil)
	require.NoError(t, err)

	project, err := store.CreateProject(ctx, org.ID, CreateProjectRequest{
		Name: "Env Project", Slug: "env-project",
	}, nil)
	require.NoError(t, err)

	env, err := store.CreateEnvironment(ctx, project.ID, CreateEnvironmentRequest{
		Name: "Production", Color: "#22c55e", Protected: true,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "Production", env.Name)
	assert.True(t, env.Protected)

	envs, err := store.ListEnvironmentsByProject(ctx, project.ID)
	require.NoError(t, err)
	assert.Len(t, envs, 1)

	updated, err := store.UpdateEnvironment(ctx, env.ID, CreateEnvironmentRequest{
		Name: "Staging", Color: "#f59e0b", Protected: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "Staging", updated.Name)

	// Deleting a non-protected env should succeed
	err = store.DeleteEnvironment(ctx, env.ID, nil)
	require.NoError(t, err)
}
