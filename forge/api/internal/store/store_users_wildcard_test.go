package store

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createWildcardTestServer(t *testing.T, s *Store, ctx context.Context, ownerID string) string {
	t.Helper()
	nodeID := uuid.NewString()
	_, err := s.DB().Exec(ctx, `
		INSERT INTO nodes (id, uuid, name, region, base_url, fqdn, scheme, status, daemon_listen, daemon_sftp, daemon_base, token_hash, last_seen_at)
		VALUES ($1, $1, 'wildcard-test-node', 'test', 'http://localhost:9090', 'localhost', 'http', 'online', 9090, 2022, '/tmp', 'test-token-hash', now())
	`, nodeID)
	require.NoError(t, err)

	var nestID string
	err = s.DB().QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID)
	require.NoError(t, err)

	eggID := uuid.NewString()
	_, err = s.DB().Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb)
		VALUES ($1, $2, $3, '', '{"test":"alpine:latest"}', '', '{}'::jsonb, 1024)
	`, eggID, nestID, "wildcard-test-egg-"+eggID)
	require.NoError(t, err)

	serverID := uuid.NewString()
	_, err = s.DB().Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, egg_id, name, status, memory_mb, cpu_shares, disk_mb)
		VALUES ($1, $2, $3, $4, $4, 'wildcard-test-server', 'stopped', 1024, 512, 2048)
	`, serverID, nodeID, ownerID, eggID)
	require.NoError(t, err)
	return serverID
}

// TestUpsertSubuser_Escalation_Rejected verifies GH-18/SE-01: a subuser holding
// only user.create cannot escalate to wildcard "*" or to sensitive permissions
// such as database.view_password that they do not themselves possess.
// Owner and admin flows must still succeed.
func TestUpsertSubuser_Escalation_Rejected(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	owner, err := s.CreateUser(ctx, CreateUserRequest{
		Email: "wildcard-owner@example.test", Password: "TestPassword123!", Role: "user",
	}, nil)
	require.NoError(t, err)

	attacker, err := s.CreateUser(ctx, CreateUserRequest{
		Email: "wildcard-attacker@example.test", Password: "TestPassword123!", Role: "user",
	}, nil)
	require.NoError(t, err)

	victim, err := s.CreateUser(ctx, CreateUserRequest{
		Email: "wildcard-victim@example.test", Password: "TestPassword123!", Role: "user",
	}, nil)
	require.NoError(t, err)

	victim2, err := s.CreateUser(ctx, CreateUserRequest{
		Email: "wildcard-victim2@example.test", Password: "TestPassword123!", Role: "user",
	}, nil)
	require.NoError(t, err)

	admin, err := s.CreateUser(ctx, CreateUserRequest{
		Email: "wildcard-admin@example.test", Password: "TestPassword123!", Role: "admin",
	}, nil)
	require.NoError(t, err)

	serverID := createWildcardTestServer(t, s, ctx, owner.ID)

	// Owner grants attacker limited perms: user.create, user.read, websocket.connect
	_, err = s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
		Email: attacker.Email, Permissions: []string{"user.create", "user.read", "websocket.connect"},
	}, &owner.ID)
	require.NoError(t, err)

	// Verify attacker effective perms are limited
	attackerSub, err := s.GetServerSubuser(ctx, serverID, attacker.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"user.create", "user.read", "websocket.connect"}, attackerSub.Permissions)

	t.Run("user.create cannot grant wildcard", func(t *testing.T) {
		_, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: victim.Email, Permissions: []string{"*"},
		}, &attacker.ID)
		require.Error(t, err)
		msg := strings.ToLower(err.Error())
		assert.Contains(t, msg, "forbidden")
		// victim must not have been created with wildcard
		_, getErr := s.GetServerSubuser(ctx, serverID, victim.ID)
		// Either not exists or not wildcard
		if getErr == nil {
			sub, _ := s.GetServerSubuser(ctx, serverID, victim.ID)
			assert.NotContains(t, sub.Permissions, "*")
		}
	})

	t.Run("user.create cannot grant database.view_password", func(t *testing.T) {
		_, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: victim2.Email, Permissions: []string{"database.view_password"},
		}, &attacker.ID)
		require.Error(t, err)
		msg := strings.ToLower(err.Error())
		assert.True(t, strings.Contains(msg, "forbidden") || strings.Contains(msg, "permission"),
			"expected forbidden/permission error, got %q", err.Error())
	})

	t.Run("user.create cannot grant permission not held (file.read)", func(t *testing.T) {
		extraVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-extra-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		_, err = s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: extraVictim.Email, Permissions: []string{"file.read"},
		}, &attacker.ID)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "forbidden")
	})

	t.Run("user.create can grant subset of own perms", func(t *testing.T) {
		subsetVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-subset-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		sub, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: subsetVictim.Email, Permissions: []string{"user.read"},
		}, &attacker.ID)
		require.NoError(t, err)
		assert.Contains(t, sub.Permissions, "user.read")
	})

	t.Run("owner can grant wildcard", func(t *testing.T) {
		ownerVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-owner-victim-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		sub, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: ownerVictim.Email, Permissions: []string{"*"},
		}, &owner.ID)
		require.NoError(t, err)
		assert.Contains(t, sub.Permissions, "*")
	})

	t.Run("admin can grant wildcard", func(t *testing.T) {
		adminVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-admin-victim-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		sub, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: adminVictim.Email, Permissions: []string{"*"},
		}, &admin.ID)
		require.NoError(t, err)
		assert.Contains(t, sub.Permissions, "*")
	})

	t.Run("admin can grant sensitive permission", func(t *testing.T) {
		sensVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-sens-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		sub, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: sensVictim.Email, Permissions: []string{"database.view_password"},
		}, &admin.ID)
		require.NoError(t, err)
		assert.Contains(t, sub.Permissions, "database.view_password")
	})

	t.Run("nil actor bypasses checks (deprecated system path)", func(t *testing.T) {
		bypassVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-bypass-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		sub, err := s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: bypassVictim.Email, Permissions: []string{"*"},
		}, nil)
		require.NoError(t, err)
		assert.Contains(t, sub.Permissions, "*")
	})

	t.Run("patch escalation also rejected", func(t *testing.T) {
		// Attacker tries to patch an existing subuser to escalate
		patchVictim, err := s.CreateUser(ctx, CreateUserRequest{
			Email: "wildcard-patch-" + uuid.NewString() + "@example.test", Password: "TestPassword123!", Role: "user",
		}, nil)
		require.NoError(t, err)
		// Owner creates patchVictim with limited perms
		_, err = s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: patchVictim.Email, Permissions: []string{"user.read"},
		}, &owner.ID)
		require.NoError(t, err)
		// Attacker attempts to patch victim to add file.sftp (which attacker doesn't have)
		_, err = s.UpsertServerSubuser(ctx, serverID, UpsertServerSubuserRequest{
			Email: patchVictim.Email, Permissions: []string{"user.read", "file.sftp"},
		}, &attacker.ID)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "forbidden")
	})
}
