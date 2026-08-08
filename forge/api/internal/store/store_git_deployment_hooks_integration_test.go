package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestGitDeploymentHooksLifecycle(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO users (id, email, username, password_hash, role)
		VALUES ($1, $2, $3, 'test-hash', 'admin')
	`, userID, userID+"@example.test", "hook-"+userID); err != nil {
		t.Fatalf("create user: %v", err)
	}

	sourceID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO git_sources (id, user_id, repository_url, repository_name)
		VALUES ($1, $2, 'https://example.test/repository.git', 'repository')
	`, sourceID, userID); err != nil {
		t.Fatalf("create git source: %v", err)
	}

	hook, err := s.CreateGitDeploymentHook(ctx, sourceID, "first-secret", []string{"push"})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}

	hooks, err := s.ListGitDeploymentHooks(ctx, sourceID)
	if err != nil {
		t.Fatalf("list hooks: %v", err)
	}
	if len(hooks) != 1 || hooks[0].ID != hook.ID {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}

	updated, err := s.RegenerateGitDeploymentHookSecret(ctx, hook.ID, "second-secret")
	if err != nil {
		t.Fatalf("regenerate secret: %v", err)
	}
	if updated.Secret != "second-secret" {
		t.Fatalf("secret was not updated")
	}

	if err := s.DeleteGitDeploymentHook(ctx, hook.ID); err != nil {
		t.Fatalf("delete hook: %v", err)
	}
	hooks, err = s.ListGitDeploymentHooks(ctx, sourceID)
	if err != nil {
		t.Fatalf("list hooks after delete: %v", err)
	}
	if len(hooks) != 0 {
		t.Fatalf("expected hook deletion, got %+v", hooks)
	}
}
