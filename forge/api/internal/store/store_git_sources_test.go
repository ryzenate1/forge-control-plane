//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestGitSourceCRUD(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	_, regionID := setupTestRegion(t, s)
	nodeID := setupTestSimpleNode(t, s, regionID)
	serverID := setupTestServerForGit(t, s, nodeID)

	t.Run("create", func(t *testing.T) {
		gs, err := s.CreateGitSource(ctx, CreateGitSourceRequest{
			ServerID:   serverID,
			RepoURL:    "https://github.com/user/repo.git",
			Branch:     "main",
			SourceType: "public",
		})
		if err != nil {
			t.Fatalf("CreateGitSource: %v", err)
		}
		if gs.ID == "" {
			t.Error("expected non-empty id")
		}
		if gs.Status != "pending" {
			t.Errorf("expected status pending, got %s", gs.Status)
		}
		if gs.RepoURL != "https://github.com/user/repo.git" {
			t.Errorf("expected repo_url 'https://github.com/user/repo.git', got %s", gs.RepoURL)
		}
	})

	t.Run("get", func(t *testing.T) {
		gs, err := s.CreateGitSource(ctx, CreateGitSourceRequest{
			ServerID:   serverID,
			RepoURL:    "https://github.com/user/repo2.git",
			Branch:     "develop",
			SourceType: "public",
		})
		if err != nil {
			t.Fatalf("CreateGitSource: %v", err)
		}

		fetched, err := s.GetGitSource(ctx, gs.ID)
		if err != nil {
			t.Fatalf("GetGitSource: %v", err)
		}
		if fetched.ID != gs.ID {
			t.Errorf("expected id %s, got %s", gs.ID, fetched.ID)
		}
		if fetched.Branch != "develop" {
			t.Errorf("expected branch develop, got %s", fetched.Branch)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		_, err := s.GetGitSource(ctx, uuid.NewString())
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})

	t.Run("list", func(t *testing.T) {
		sources, err := s.ListGitSources(ctx, serverID)
		if err != nil {
			t.Fatalf("ListGitSources: %v", err)
		}
		if len(sources) < 2 {
			t.Errorf("expected at least 2 sources, got %d", len(sources))
		}
	})

	t.Run("update", func(t *testing.T) {
		gs, err := s.CreateGitSource(ctx, CreateGitSourceRequest{
			ServerID:   serverID,
			RepoURL:    "https://github.com/user/repo3.git",
			Branch:     "main",
			SourceType: "public",
		})
		if err != nil {
			t.Fatalf("CreateGitSource: %v", err)
		}

		gs.CommitSHA = "abc123def"
		gs.Status = "cloned"
		gs.CloneDir = "/tmp/clones/abc"
		gs.BuildContextSizeBytes = 1024

		if err := s.UpdateGitSource(ctx, gs); err != nil {
			t.Fatalf("UpdateGitSource: %v", err)
		}

		fetched, err := s.GetGitSource(ctx, gs.ID)
		if err != nil {
			t.Fatalf("GetGitSource after update: %v", err)
		}
		if fetched.CommitSHA != "abc123def" {
			t.Errorf("expected commit_sha 'abc123def', got %s", fetched.CommitSHA)
		}
		if fetched.Status != "cloned" {
			t.Errorf("expected status 'cloned', got %s", fetched.Status)
		}
	})

	t.Run("update status", func(t *testing.T) {
		gs, err := s.CreateGitSource(ctx, CreateGitSourceRequest{
			ServerID:   serverID,
			RepoURL:    "https://github.com/user/repo4.git",
			Branch:     "main",
			SourceType: "public",
		})
		if err != nil {
			t.Fatalf("CreateGitSource: %v", err)
		}

		if err := s.UpdateGitSourceStatus(ctx, gs.ID, "failed", "clone timed out"); err != nil {
			t.Fatalf("UpdateGitSourceStatus: %v", err)
		}

		fetched, err := s.GetGitSource(ctx, gs.ID)
		if err != nil {
			t.Fatalf("GetGitSource after status update: %v", err)
		}
		if fetched.Status != "failed" {
			t.Errorf("expected status 'failed', got %s", fetched.Status)
		}
		if fetched.Error != "clone timed out" {
			t.Errorf("expected error 'clone timed out', got %s", fetched.Error)
		}
	})

	t.Run("delete", func(t *testing.T) {
		gs, err := s.CreateGitSource(ctx, CreateGitSourceRequest{
			ServerID:   serverID,
			RepoURL:    "https://github.com/user/repo5.git",
			Branch:     "main",
			SourceType: "public",
		})
		if err != nil {
			t.Fatalf("CreateGitSource: %v", err)
		}

		if err := s.DeleteGitSource(ctx, gs.ID); err != nil {
			t.Fatalf("DeleteGitSource: %v", err)
		}

		_, err = s.GetGitSource(ctx, gs.ID)
		if err == nil {
			t.Error("expected error after delete")
		}
	})
}

func setupTestServerForGit(t *testing.T, s *Store, nodeID string) string {
	t.Helper()
	ctx := context.Background()
	userID := setupTestUser(t, s)
	eggID := setupTestEggForGit(t, s)

	serverID := uuid.NewString()
	if err := s.CreateServer(ctx, Server{
		ID:       serverID,
		Name:     "git-test-server",
		NodeID:   nodeID,
		OwnerID:  userID,
		EggID:    eggID,
		Status:   "installing",
		MemoryMB: 1024,
		DiskMB:   10240,
	}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	return serverID
}

func setupTestEggForGit(t *testing.T, s *Store) string {
	t.Helper()
	eggID := uuid.NewString()
	nestID := uuid.NewString()

	_, err := s.db.Exec(context.Background(), `
		INSERT INTO nests (id, name, description, created_at, updated_at)
		VALUES ($1, 'test-nest', 'test', now(), now())
		ON CONFLICT DO NOTHING
	`, nestID)
	if err != nil {
		t.Fatalf("insert nest: %v", err)
	}

	_, err = s.db.Exec(context.Background(), `
		INSERT INTO eggs (id, nest_id, name, description, startup, docker_images, created_at, updated_at)
		VALUES ($1, $2, 'test-egg', 'test egg', 'echo start', '{}', now(), now())
		ON CONFLICT DO NOTHING
	`, eggID, nestID)
	if err != nil {
		t.Fatalf("insert egg: %v", err)
	}

	return eggID
}
