package compose

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitOps_DeployFromGitValidation validates the deploy request.
func TestGitOps_DeployFromGitValidation(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	ctx := context.Background()

	_, err := svc.DeployFromGit(ctx, GitDeployFromGitRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	_, err = svc.DeployFromGit(ctx, GitDeployFromGitRequest{
		UserID: "u1", NodeID: "n1", Name: "test", RepositoryURL: "https://github.com/user/repo.git",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

// TestGitOps_NonGitBackedErrors checks that non-git stacks return ErrStackNotGitBacked.
func TestGitOps_NonGitBackedErrors(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	ctx := context.Background()

	_, err := svc.CheckForUpdates(ctx, "nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.RedeployFromGit(ctx, "nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.RollbackToPrevious(ctx, "nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.DetectDrift(ctx, "nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.PullAndRedeploy(ctx, "nonexistent")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.SetBranch(ctx, "nonexistent", "main")
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)

	_, err = svc.SetAutoUpdate(ctx, "nonexistent", true, 300)
	require.Error(t, err)
	assert.Equal(t, ErrStackNotFound, err)
}

// TestGitOps_StackNotGitBacked checks operations on non-git stacks.
func TestGitOps_StackNotGitBacked(t *testing.T) {
	st := &Service{store: nil}
	svc := NewGitOpsService(nil, st, nil, nil, nil)
	ctx := context.Background()

	gitBackedOps := []func() error{
		func() error { _, err := svc.CheckForUpdates(ctx, "cps-abc"); return err },
		func() error { _, err := svc.GetUpdatePreview(ctx, "cps-abc"); return err },
		func() error { _, err := svc.RedeployFromGit(ctx, "cps-abc"); return err },
		func() error { _, err := svc.DetectDrift(ctx, "cps-abc"); return err },
		func() error { _, err := svc.PullAndRedeploy(ctx, "cps-abc"); return err },
		func() error { _, err := svc.SetBranch(ctx, "cps-abc", "main"); return err },
		func() error { _, err := svc.RollbackToPrevious(ctx, "cps-abc"); return err },
	}
	for _, op := range gitBackedOps {
		err := op()
		require.Error(t, err)
	}
}

// TestGitOps_WebhookHMACSignature verifies signature verification.
func TestGitOps_WebhookHMACSignature(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-test"
	webhookID := "wh-abc123"
	secret := "my-webhook-secret"

	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitWebhookSecret: secret,
		GitWebhookID:     webhookID,
		GitBranch:        "main",
		GitCommitSHA:     "abc123",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	payload := []byte(`{"ref":"refs/heads/main","after":"def456","head_commit":{"message":"fix","author":{"name":"dev"}}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	accepted, err := svc.HandleWebhook(ctx, webhookID, payload, sig, "delivery-1")
	require.NoError(t, err)
	assert.True(t, accepted)
}

// TestGitOps_WebhookBadSignature rejects bad signatures.
func TestGitOps_WebhookBadSignature(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-badsig"
	webhookID := "wh-badsig"

	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitWebhookSecret: "correct-secret",
		GitWebhookID:     webhookID,
		GitBranch:        "main",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	payload := []byte(`{"ref":"refs/heads/main","after":"def456","head_commit":{"message":"fix","author":{"name":"dev"}}}`)

	_, err := svc.HandleWebhook(ctx, webhookID, payload, "sha256=wrongsignature", "delivery-2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

// TestGitOps_WebhookStaleRejection rejects old/stale webhooks.
func TestGitOps_WebhookStaleRejection(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-stale"
	webhookID := "wh-stale"
	secret := "test-secret"

	stack := &ComposeStack{
		ID:                  stackID,
		GitRepositoryURL:    "https://github.com/user/repo.git",
		GitWebhookSecret:    secret,
		GitWebhookID:        webhookID,
		GitBranch:           "main",
		GitCommitSHA:        "def789",
		GitDesiredCommitSHA: "def789",
		Status:              StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	payload := []byte(`{"ref":"refs/heads/main","after":"def789","head_commit":{"message":"same","author":{"name":"dev"}}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	_, err := svc.HandleWebhook(ctx, webhookID, payload, sig, "delivery-3")
	require.Error(t, err)
	assert.Equal(t, ErrWebhookStale, err)
}

// TestGitOps_WebhookWrongBranch ignores webhook for wrong branch.
func TestGitOps_WebhookWrongBranch(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-wrong-branch"
	webhookID := "wh-wbranch"
	secret := "test-secret"

	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitWebhookSecret: secret,
		GitWebhookID:     webhookID,
		GitBranch:        "main",
		GitCommitSHA:     "abc111",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	payload := []byte(`{"ref":"refs/heads/develop","after":"def222","head_commit":{"message":"dev update","author":{"name":"dev"}}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	accepted, err := svc.HandleWebhook(ctx, webhookID, payload, sig, "delivery-4")
	require.NoError(t, err)
	assert.False(t, accepted)
}

// TestGitOps_SetBranch updates branch and resets commit tracking.
func TestGitOps_SetBranch(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-setbranch"
	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitBranch:        "main",
		GitCommitSHA:     "abc123",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)

	_, err := svc.SetBranch(ctx, stackID, "develop")
	require.NoError(t, err)
	s := store.getStack(stackID)
	require.NotNil(t, s)
	assert.Equal(t, "develop", s.GitBranch)
	assert.Empty(t, s.GitCommitSHA)
	assert.Equal(t, "branch_changed", s.GitUpdateStatus)
}

// TestGitOps_SetBranchInvalid rejects invalid branch names.
func TestGitOps_SetBranchInvalid(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	ctx := context.Background()
	_, err := svc.SetBranch(ctx, "any", "../evil")
	require.Error(t, err)
}

// TestGitOps_RollbackToPreviousNoDeployment returns error when no previous.
func TestGitOps_RollbackToPreviousNoDeployment(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-norollback"
	stack := &ComposeStack{
		ID:                   stackID,
		GitRepositoryURL:     "https://github.com/user/repo.git",
		GitBranch:            "main",
		GitCommitSHA:         "abc123",
		GitPreviousCommitSHA: "",
		GitPreviousCompose:   "",
		Status:               StackStatusRunning,
	}
	store.setStack(stackID, stack)

	_, err := svc.RollbackToPrevious(ctx, stackID)
	require.Error(t, err)
	assert.Equal(t, ErrNoPreviousDeployment, err)
}

// TestGitOps_ComputeHash verifies hash consistency.
func TestGitOps_ComputeHash(t *testing.T) {
	a := computeHash("version: '3'\nservices:\n  web:\n    image: nginx")
	b := computeHash("version: '3'\nservices:\n  web:\n    image: nginx")
	c := computeHash("version: '3'\nservices:\n  web:\n    image: nginx:alpine")

	assert.Equal(t, a, b)
	assert.NotEqual(t, a, c)
}

// TestGitOps_GetGitStatus returns status for non-git stacks.
func TestGitOps_GetGitStatus(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-gitstatus"
	stack := &ComposeStack{
		ID:                 stackID,
		GitRepositoryURL:   "https://github.com/user/repo.git",
		GitBranch:          "main",
		GitCommitSHA:       "abc123",
		GitAutoUpdate:      true,
		GitPollIntervalSec: 600,
		GitUpdateStatus:    "idle",
		Status:             StackStatusRunning,
	}
	store.setStack(stackID, stack)

	status, err := svc.GetGitStatus(ctx, stackID)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/user/repo.git", status["gitRepositoryUrl"])
	assert.Equal(t, "main", status["gitBranch"])
	assert.Equal(t, "abc123", status["gitCommitSha"])
	assert.Equal(t, true, status["gitAutoUpdate"])
	assert.Equal(t, float64(600), status["gitPollIntervalSec"])
	assert.Equal(t, "idle", status["gitUpdateStatus"])
}

// TestGitOps_SetAutoUpdate toggles polling.
func TestGitOps_SetAutoUpdate(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-autoupdate"
	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitBranch:        "main",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)

	r, err := svc.SetAutoUpdate(ctx, stackID, true, 120)
	require.NoError(t, err)
	assert.True(t, r.GitAutoUpdate)
	assert.Equal(t, 120, r.GitPollIntervalSec)

	r2, err := svc.SetAutoUpdate(ctx, stackID, false, 0)
	require.NoError(t, err)
	assert.False(t, r2.GitAutoUpdate)
}

// TestGitOps_DetectDrift on non-git stack returns error.
func TestGitOps_DetectDriftNonGit(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-drift-nongit"
	stack := &ComposeStack{
		ID:     stackID,
		Status: StackStatusRunning,
	}
	store.setStack(stackID, stack)

	_, err := svc.DetectDrift(ctx, stackID)
	require.Error(t, err)
	assert.Equal(t, ErrStackNotGitBacked, err)
}

// TestGitOps_LoadAndParseComposeFromDisk validates reading compose from disk.
func TestGitOps_LoadAndParseComposeFromDisk(t *testing.T) {
	dir := t.TempDir()
	composeContent := `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`
	err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(composeContent), 0644)
	require.NoError(t, err)

	parsed, err := ParseSummary([]byte(composeContent), dir)
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "web", parsed.Services[0].Name)
	assert.Equal(t, "nginx:latest", parsed.Services[0].Image)
}

// TestGitOps_WebhookNoSignature returns error on missing signature.
func TestGitOps_WebhookNoSignature(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-nosig"
	webhookID := "wh-nosig"

	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitWebhookSecret: "secret",
		GitWebhookID:     webhookID,
		GitBranch:        "main",
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	_, err := svc.HandleWebhook(ctx, webhookID, []byte(`{}`), "", "delivery-5")
	require.Error(t, err)
}

// TestGitOps_WebhookNoSecret returns error when no secret.
func TestGitOps_WebhookNoSecret(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	webhookID := "wh-nosecret"
	stack := &ComposeStack{
		ID:           "cps-nosec",
		GitWebhookID: webhookID,
		Status:       StackStatusRunning,
	}
	store.setStack("cps-nosec", stack)
	store.setWebhookID(webhookID, "cps-nosec")

	_, err := svc.HandleWebhook(ctx, webhookID, []byte(`{}`), "sha256=abc", "delivery-6")
	require.Error(t, err)
}

// TestGitOps_GetLastWebhookAt returns nil for no webhook.
func TestGitOps_GetLastWebhookAt(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-lastwh"
	stack := &ComposeStack{
		ID:     stackID,
		Status: StackStatusRunning,
	}
	store.setStack(stackID, stack)

	lastAt, err := svc.GetLastWebhookAt(ctx, stackID)
	require.NoError(t, err)
	assert.Nil(t, lastAt)

	now := time.Now().UTC()
	stack.GitLastWebhookAt = &now
	store.setStack(stackID, stack)

	lastAt, err = svc.GetLastWebhookAt(ctx, stackID)
	require.NoError(t, err)
	require.NotNil(t, lastAt)
	assert.WithinDuration(t, now, *lastAt, time.Second)
}

// TestGitOps_WebhookDuplicateIgnore ignores duplicate commits.
func TestGitOps_WebhookDuplicateCommit(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-webhook-dup"
	webhookID := "wh-dup"
	secret := "test-secret"
	commitSHA := "def456def456def456def456def456def456def4"

	stack := &ComposeStack{
		ID:               stackID,
		GitRepositoryURL: "https://github.com/user/repo.git",
		GitWebhookSecret: secret,
		GitWebhookID:     webhookID,
		GitBranch:        "main",
		GitCommitSHA:     commitSHA,
		Status:           StackStatusRunning,
	}
	store.setStack(stackID, stack)
	store.setWebhookID(webhookID, stackID)

	payload := []byte(`{"ref":"refs/heads/main","after":"` + commitSHA + `","head_commit":{"message":"same","author":{"name":"dev"}}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	_, err := svc.HandleWebhook(ctx, webhookID, payload, sig, "delivery-7")
	require.Error(t, err)
}

// TestGitOps_SetWebhookSecret stores secret and ID.
func TestGitOps_SetWebhookSecret(t *testing.T) {
	svc := NewGitOpsService(nil, nil, nil, nil, nil)
	store := &mockStore{stacks: map[string]*store.ComposeStack{}}
	svc.store = store
	ctx := context.Background()

	stackID := "cps-setwhsec"
	stack := &ComposeStack{ID: stackID, Status: StackStatusRunning}
	store.setStack(stackID, stack)

	err := svc.SetWebhookSecret(ctx, stackID, "newsecret", "wh-123")
	require.NoError(t, err)

	s := store.getStack(stackID)
	assert.Equal(t, "newsecret", s.GitWebhookSecret)
	assert.Equal(t, "wh-123", s.GitWebhookID)
}

// --- Mock store for testing ---

type mockStore struct {
	stacks    map[string]*store.ComposeStack
	webhookID map[string]string
}

func (m *mockStore) setStack(id string, s *ComposeStack) {
	if m.stacks == nil {
		m.stacks = make(map[string]*store.ComposeStack)
	}
	m.stacks[id] = toStoreComposeStack(s)
}

func (m *mockStore) getStack(id string) *ComposeStack {
	if m.stacks == nil {
		return nil
	}
	s, ok := m.stacks[id]
	if !ok {
		return nil
	}
	return fromStoreComposeStack(*s)
}

func (m *mockStore) setWebhookID(whID, stackID string) {
	if m.webhookID == nil {
		m.webhookID = make(map[string]string)
	}
	m.webhookID[whID] = stackID
}

func (m *mockStore) GetComposeStack(ctx context.Context, id string) (store.ComposeStack, error) {
	s, ok := m.stacks[id]
	if !ok {
		return store.ComposeStack{}, fmt.Errorf("not found")
	}
	return *s, nil
}

func (m *mockStore) UpdateComposeStack(ctx context.Context, s *store.ComposeStack) error {
	m.stacks[s.ID] = s
	return nil
}

func (m *mockStore) GetComposeStackByWebhookID(ctx context.Context, webhookID string) (*store.ComposeStack, error) {
	stackID, ok := m.webhookID[webhookID]
	if !ok {
		return nil, nil
	}
	s := m.getStack(stackID)
	if s == nil {
		return nil, nil
	}
	return toStoreComposeStack(s), nil
}

func (m *mockStore) ListComposeStacksDueForPoll(ctx context.Context) ([]store.ComposeStack, error) {
	return nil, nil
}

func (m *mockStore) GetGitCredential(ctx context.Context, id string) (store.GitCredential, error) {
	return store.GitCredential{}, nil
}

func (m *mockStore) GetNode(ctx context.Context, id string) (store.Node, error) {
	return store.Node{}, nil
}

func (m *mockStore) GetNodeDaemonCredential(ctx context.Context, nodeID string) (string, error) {
	return "", nil
}

func (m *mockStore) CreatePlacementReservation(ctx context.Context, req store.CreatePlacementReservationRequest) (store.PlacementReservation, error) {
	return store.PlacementReservation{ID: "res-1"}, nil
}

func (m *mockStore) UpdatePlacementReservationStatus(ctx context.Context, id string, status store.PlacementReservationStatus) (store.PlacementReservation, error) {
	return store.PlacementReservation{}, nil
}

func (m *mockStore) ClaimComposeStackForUpdate(ctx context.Context, stackID, workerID string) (*store.ComposeStack, error) {
	s, ok := m.stacks[stackID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	s.GitUpdateStatus = "deploying"
	s.GitUpdateClaimedBy = strPtr(workerID)
	return s, nil
}

func (m *mockStore) ReleaseComposeStackClaim(ctx context.Context, stackID, workerID string) error {
	return nil
}

func (m *mockStore) ListComposeStacksPendingUpdate(ctx context.Context) ([]store.ComposeStack, error) {
	return nil, nil
}

func (m *mockStore) ListComposeStacksStaleClaims(ctx context.Context, staleDuration time.Duration) ([]store.ComposeStack, error) {
	return nil, nil
}

func (m *mockStore) ListComposeStacksForReconciliation(ctx context.Context) ([]store.ComposeStack, error) {
	return nil, nil
}

func (m *mockStore) DispatchWebhookEvent(event string, payload map[string]any) {
}

func strPtr(s string) *string { return &s }
