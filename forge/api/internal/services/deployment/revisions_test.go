package deployment

import (
	"context"
	"encoding/json"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRevision(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	rev, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, rev.ID)
	assert.Equal(t, 1, rev.RevisionNumber)
	assert.Equal(t, "nginx:1.25", rev.ImageRef)
	assert.Equal(t, string(store.RevisionStatusPending), rev.Status)
	assert.NotEmpty(t, rev.ConfigHash)
}

func TestListRevisions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	_, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	})
	require.NoError(t, err)

	_, err = svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26",
		Description: "version bump",
	})
	require.NoError(t, err)

	revs, err := svc.ListRevisions(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Len(t, revs, 2)
	assert.Equal(t, 2, revs[0].RevisionNumber)
	assert.Equal(t, 1, revs[1].RevisionNumber)
}

func TestGetRevision(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	rev, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	})
	require.NoError(t, err)

	fetched, err := svc.GetRevision(context.Background(), rev.ID)
	require.NoError(t, err)
	assert.Equal(t, rev.ID, fetched.ID)
	assert.Equal(t, rev.RevisionNumber, fetched.RevisionNumber)
}

func TestGetRevisionNotFound(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	_, err := svc.GetRevision(context.Background(), "nonexistent-id")
	assert.ErrorIs(t, err, ErrRevisionNotFound)
}

func TestRollbackToRevision(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	rev1, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	})
	require.NoError(t, err)

	rev2, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26",
		Description: "version bump",
	})
	require.NoError(t, err)

	err = svc.store.UpdateDeploymentCurrentRevision(context.Background(), dep.ID, &rev2.ID)
	require.NoError(t, err)

	result, err := svc.RollbackToRevision(context.Background(), dep.ID, rev1.ID)
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.25", result.Image)
	assert.Equal(t, StatusInProgress, result.Status)
}

func TestRollbackToPrevious(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	_, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	})
	require.NoError(t, err)

	rev2, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26",
		Description: "version bump",
	})
	require.NoError(t, err)

	err = svc.store.UpdateDeploymentCurrentRevision(context.Background(), dep.ID, &rev2.ID)
	require.NoError(t, err)

	result, err := svc.RollbackToPrevious(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "nginx:1.25", result.Image)
}

func TestRollbackToPreviousNoRevisions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	_, err := svc.RollbackToPrevious(context.Background(), dep.ID)
	assert.ErrorIs(t, err, ErrNoRevisions)
}

func TestCompareRevisions(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	rev1, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
		GitCommitSHA: "abc123",
	})
	require.NoError(t, err)

	rev2, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26",
		Description: "version bump",
		GitCommitSHA: "def456",
	})
	require.NoError(t, err)

	diff, err := svc.CompareRevisions(context.Background(), rev1.ID, rev2.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, diff.FromRevisionID)
	assert.Equal(t, 2, diff.ToRevisionID)
	assert.NotEmpty(t, diff.Changes)

	var foundImage, foundGit bool
	for _, ch := range diff.Changes {
		if ch.Field == "imageRef" {
			assert.Equal(t, "nginx:1.25", ch.OldValue)
			assert.Equal(t, "nginx:1.26", ch.NewValue)
			foundImage = true
		}
		if ch.Field == "gitCommitSha" {
			assert.Equal(t, "abc123", ch.OldValue)
			assert.Equal(t, "def456", ch.NewValue)
			foundGit = true
		}
	}
	assert.True(t, foundImage, "should find imageRef change")
	assert.True(t, foundGit, "should find gitCommitSha change")
}

func TestCompareRevisionsNoChanges(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	cfg := &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial deploy",
	}

	rev1, err := svc.CreateRevision(context.Background(), dep.ID, cfg)
	require.NoError(t, err)
	rev2, err := svc.CreateRevision(context.Background(), dep.ID, cfg)
	require.NoError(t, err)

	diff, err := svc.CompareRevisions(context.Background(), rev1.ID, rev2.ID)
	require.NoError(t, err)
	assert.Empty(t, diff.Changes)
}

func TestConfigHash(t *testing.T) {
	cfg1 := &RevisionConfig{ImageRef: "nginx:1.25", GitCommitSHA: "abc123"}
	cfg2 := &RevisionConfig{ImageRef: "nginx:1.25", GitCommitSHA: "abc123"}
	cfg3 := &RevisionConfig{ImageRef: "nginx:1.26", GitCommitSHA: "abc123"}

	h1 := configHash(cfg1)
	h2 := configHash(cfg2)
	h3 := configHash(cfg3)

	assert.Equal(t, h1, h2, "identical configs should produce same hash")
	assert.NotEqual(t, h1, h3, "different configs should produce different hashes")
	assert.Len(t, h1, 12, "hash should be 12 chars")
}

func TestCreateRevisionRevisionNumberSequence(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyRecreate)

	for i := 1; i <= 5; i++ {
		rev, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
			ImageRef:    "nginx:latest",
			Description: "revision " + string(rune('0'+i)),
		})
		require.NoError(t, err)
		assert.Equal(t, i, rev.RevisionNumber)
	}
}

func TestCreateRevisionDeploymentNotFound(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	_, err := svc.CreateRevision(context.Background(), "nonexistent", &RevisionConfig{
		ImageRef: "nginx:latest",
	})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRollbackToRevisionMismatchedDeployment(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep1 := createTestDeployment(t, svc, "test-server-1", StrategyRecreate)
	dep2 := createTestDeployment(t, svc, "test-server-2", StrategyRecreate)

	rev, err := svc.CreateRevision(context.Background(), dep1.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "initial",
	})
	require.NoError(t, err)

	_, err = svc.RollbackToRevision(context.Background(), dep2.ID, rev.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to this deployment")
}

func TestRevisionMetadata(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestDeployment(t, svc, "test-server-id", StrategyCanary)

	rev, err := svc.CreateRevision(context.Background(), dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25",
		Description: "canary deploy",
		Metadata: map[string]any{
			"canaryPercent": 10,
			"env":           "staging",
		},
	})
	require.NoError(t, err)

	var meta map[string]any
	err = json.Unmarshal(rev.Metadata, &meta)
	require.NoError(t, err)
	assert.Equal(t, float64(10), meta["canaryPercent"])
	assert.Equal(t, "staging", meta["env"])
}

func TestStartRollout(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	tests := []struct {
		name     string
		strategy Strategy
	}{
		{name: "recreate", strategy: StrategyRecreate},
		{name: "rolling", strategy: StrategyRolling},
		{name: "canary", strategy: StrategyCanary},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep, err := svc.StartRollout(context.Background(), &RolloutRequest{
				ServerID: "test-server-" + tt.name,
				Strategy: tt.strategy,
				Image:    "nginx:latest",
			})
			require.NoError(t, err)
			assert.Equal(t, tt.strategy, dep.Strategy)
			assert.Equal(t, StatusInProgress, dep.Status)
			assert.NotEmpty(t, dep.ID)
		})
	}
}

func TestStartRolloutDuplicateInProgress(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	_, err := svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID: "test-server-dup",
		Strategy: StrategyRecreate,
		Image:    "nginx:latest",
	})
	require.NoError(t, err)

	_, err = svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID: "test-server-dup",
		Strategy: StrategyRecreate,
		Image:    "nginx:latest",
	})
	assert.ErrorIs(t, err, ErrInProgress)
}

func TestStartRolloutInvalidInput(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	_, err := svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID: "",
		Strategy: StrategyRecreate,
	})
	assert.ErrorIs(t, err, ErrInvalidServer)

	_, err = svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID: "test-server",
		Strategy: StrategyRecreate,
		Image:    "",
	})
	assert.ErrorIs(t, err, ErrInvalidImage)
}

func TestStartRolloutBlueGreen(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep, err := svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID:        "test-server-bg",
		Strategy:        StrategyBlueGreen,
		Image:           "nginx:latest",
		HealthCheckPath: "/health",
		HealthCheckPort: 8080,
	})
	require.NoError(t, err)
	assert.Equal(t, StrategyBlueGreen, dep.Strategy)
	assert.NotEmpty(t, dep.BlueTargetID)
	assert.NotEmpty(t, dep.GreenTargetID)
	assert.Equal(t, "blue", dep.ActiveTarget)
}

func TestCanaryRolloutDefaultPercent(t *testing.T) {
	svc, cleanup := testService(t)
	defer cleanup()

	dep, err := svc.StartRollout(context.Background(), &RolloutRequest{
		ServerID: "test-server-canary",
		Strategy: StrategyCanary,
		Image:    "nginx:latest",
	})
	require.NoError(t, err)
	assert.Equal(t, StrategyCanary, dep.Strategy)
	assert.Equal(t, "blue", dep.ActiveTarget)
}
