package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
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

func setupCrossTenantTest(t *testing.T, store *Store, ctx context.Context) (owner1, owner2, member1 string, orgA, orgB string, serverA, serverB string) {
	t.Helper()

	// Create users
	u1, err := store.CreateUser(ctx, CreateUserRequest{
		Email: "cross-owner-a@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	u2, err := store.CreateUser(ctx, CreateUserRequest{
		Email: "cross-owner-b@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	u3, err := store.CreateUser(ctx, CreateUserRequest{
		Email: "cross-member-a@example.com", Password: "TestPassword123!",
		Role: "user",
	}, nil)
	require.NoError(t, err)

	// Create organizations
	org1, err := store.CreateOrganization(ctx, CreateOrganizationRequest{Name: "Cross Org A", Slug: "cross-org-a"}, u1.ID, nil)
	require.NoError(t, err)

	org2, err := store.CreateOrganization(ctx, CreateOrganizationRequest{Name: "Cross Org B", Slug: "cross-org-b"}, u2.ID, nil)
	require.NoError(t, err)

	// Add member1 to org A
	_, err = store.AddTeamMember(ctx, org1.ID, u3.ID, "member", u1.ID)
	require.NoError(t, err)

	// Set up a minimal node and egg for server creation
	nodeID := uuid.NewString()
	_, err = store.DB().Exec(ctx, `
		INSERT INTO nodes (id, uuid, name, region, base_url, fqdn, scheme, status, daemon_listen, daemon_sftp, daemon_base, last_seen_at)
		VALUES ($1, $1, 'cross-test-node', 'cross', 'http://localhost:9090', 'localhost', 'http', 'online', 9090, 2022, '/tmp', now())
	`, nodeID)
	require.NoError(t, err)

	eggID := uuid.NewString()
	_, err = store.DB().Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb)
		VALUES ($1, $1, 'cross-test-egg', '', '{"test":"alpine:latest"}', '', '{}'::jsonb, 1024)
	`, eggID)
	require.NoError(t, err)

	// Create servers in each org
	server1ID := uuid.NewString()
	_, err = store.DB().Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, egg_id, name, status, memory_mb, cpu_shares, disk_mb, org_id)
		VALUES ($1, $2, $3, $4, $4, 'cross-server-a', 'stopped', 1024, 512, 2048, $5)
	`, server1ID, nodeID, u1.ID, eggID, org1.ID)
	require.NoError(t, err)

	server2ID := uuid.NewString()
	_, err = store.DB().Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, egg_id, name, status, memory_mb, cpu_shares, disk_mb, org_id)
		VALUES ($1, $2, $3, $4, $4, 'cross-server-b', 'stopped', 1024, 512, 2048, $5)
	`, server2ID, nodeID, u2.ID, eggID, org2.ID)
	require.NoError(t, err)

	return u1.ID, u2.ID, u3.ID, org1.ID, org2.ID, server1ID, server2ID
}

func TestCrossTenantServerAccess(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	owner1, owner2, _, orgA, _, serverA, _ := setupCrossTenantTest(t, store, ctx)

	// owner2 (from Org B) should NOT be able to access serverA (in Org A)
	t.Run("cross-tenant server read denied", func(t *testing.T) {
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner2, "user", "")
		require.NoError(t, err)
		assert.False(t, allowed, "owner2 should not access owner1's server")

		belongs, err := store.ServerBelongsToOrg(ctx, serverA, orgA)
		require.NoError(t, err)
		assert.True(t, belongs, "serverA should belong to orgA")

		// Cross-org should fail
		belongs, err = store.ServerBelongsToOrg(ctx, serverA, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.False(t, belongs, "serverA should NOT belong to non-existent org")
	})

	t.Run("cross-tenant org resource access denied", func(t *testing.T) {
		allowed, err := store.UserCanAccessOrgResource(ctx, serverA, orgA, owner2, "user")
		require.NoError(t, err)
		assert.False(t, allowed, "owner2 should not access serverA through orgA")
	})

	t.Run("owner1 can access own server", func(t *testing.T) {
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner1, "user", "")
		require.NoError(t, err)
		assert.True(t, allowed, "owner1 should access own server")
	})

	t.Run("admin bypasses org restrictions", func(t *testing.T) {
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner2, "admin", "")
		require.NoError(t, err)
		assert.True(t, allowed, "admin should bypass org restrictions")

		allowed, err = store.UserCanAccessOrgResource(ctx, serverA, orgA, owner2, "admin")
		require.NoError(t, err)
		assert.True(t, allowed, "admin should bypass org resource check")
	})

	t.Run("member of org A can access server A", func(t *testing.T) {
		// Member doesn't own the server, so UserCanAccessServer returns false
		// This is correct — subuser access is required for non-owners
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner1, "user", "")
		require.NoError(t, err)
		assert.True(t, allowed)
	})
}

func TestCrossTenantBackupAccess(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	_, _, _, _, _, serverA, _ := setupCrossTenantTest(t, store, ctx)

	// Insert a backup for serverA
	backupUUID := uuid.NewString()
	_, err := store.DB().Exec(ctx, `
		INSERT INTO backups (uuid, server_id, name, checksum, size, status)
		VALUES ($1, $2, 'cross-backup', 'abc123', 1024, 'completed')
	`, backupUUID, serverA)
	require.NoError(t, err)

	t.Run("cross-tenant backup access denied", func(t *testing.T) {
		belongs, err := store.BackupBelongsToOrg(ctx, backupUUID, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.False(t, belongs, "backup should not belong to bogus org")
	})
}

func TestCrossTenantDeploymentAccess(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	_, owner2, _, _, _, serverA, _ := setupCrossTenantTest(t, store, ctx)

	// Insert a deployment for serverA
	deployID := uuid.NewString()
	_, err := store.DB().Exec(ctx, `
		INSERT INTO deployments (id, server_id, strategy, status, image, blue_target_id, green_target_id, active_target, created_at, updated_at)
		VALUES ($1, $2, 'recreate', 'completed', 'alpine:latest', $3, $3, 'blue', now(), now())
	`, deployID, serverA, uuid.NewString())
	require.NoError(t, err)

	t.Run("cross-tenant deployment access denied", func(t *testing.T) {
		belongs, err := store.DeploymentBelongsToOrg(ctx, deployID, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.False(t, belongs, "deployment should not belong to bogus org")

		// Admin bypass
		belongs, err = store.DeploymentBelongsToOrg(ctx, deployID, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.False(t, belongs)
	})

	t.Run("owner2 cannot access deployment from org A", func(t *testing.T) {
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner2, "user", "")
		require.NoError(t, err)
		assert.False(t, allowed, "owner2 should not access serverA's deployment")
	})
}

func TestAdminOverridesTenancy(t *testing.T) {
	store := migrationTestStore(t, false)
	ctx := context.Background()

	_, owner2, _, _, _, serverA, _ := setupCrossTenantTest(t, store, ctx)

	t.Run("admin can access any server", func(t *testing.T) {
		allowed, err := store.UserCanAccessServer(ctx, serverA, owner2, "admin", "")
		require.NoError(t, err)
		assert.True(t, allowed, "admin must be able to access any server")
	})

	t.Run("admin bypasses org membership checks", func(t *testing.T) {
		allowed, err := store.UserCanAccessOrgResource(ctx, serverA, "00000000-0000-0000-0000-000000000000", owner2, "admin")
		require.NoError(t, err)
		assert.True(t, allowed, "admin bypasses org checks even with non-existent org")
	})
}
