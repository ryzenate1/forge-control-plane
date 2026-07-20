package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type RevisionConfig struct {
	ImageRef           string `json:"imageRef"`
	ComposeManifestRef string `json:"composeManifestRef"`
	GitCommitSHA       string `json:"gitCommitSha"`
	Description        string `json:"description"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type Revision struct {
	ID                 string          `json:"id"`
	DeploymentID       string          `json:"deploymentId"`
	RevisionNumber     int             `json:"revisionNumber"`
	ImageRef           string          `json:"imageRef"`
	ComposeManifestRef string          `json:"composeManifestRef"`
	GitCommitSHA       string          `json:"gitCommitSha"`
	ConfigHash         string          `json:"configHash"`
	Status             string          `json:"status"`
	DeployedAt         *time.Time      `json:"deployedAt,omitempty"`
	Description        string          `json:"description"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

type RevisionDiff struct {
	FromRevisionID int               `json:"fromRevisionId"`
	ToRevisionID   int               `json:"toRevisionId"`
	Changes        []RevisionChange  `json:"changes"`
}

type RevisionChange struct {
	Field    string `json:"field"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
}

var (
	ErrRevisionNotFound = fmt.Errorf("revision not found")
	ErrNoRevisions      = fmt.Errorf("no revisions available")
)

func configHash(cfg *RevisionConfig) string {
	raw, _ := json.Marshal(cfg)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])[:12]
}

func toRevision(r store.DeploymentRevision) *Revision {
	return &Revision{
		ID:                 r.ID,
		DeploymentID:       r.DeploymentID,
		RevisionNumber:     r.RevisionNumber,
		ImageRef:           r.ImageRef,
		ComposeManifestRef: r.ComposeManifestRef,
		GitCommitSHA:       r.GitCommitSHA,
		ConfigHash:         r.ConfigHash,
		Status:             r.Status,
		DeployedAt:         r.DeployedAt,
		Description:        r.Description,
		Metadata:           r.Metadata,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

func (s *Service) CreateRevision(ctx context.Context, deploymentID string, cfg *RevisionConfig) (*Revision, error) {
	if _, err := s.store.GetDeployment(ctx, deploymentID); err != nil {
		return nil, ErrNotFound
	}

	latest, err := s.store.GetLatestDeploymentRevision(ctx, deploymentID)
	nextNum := 1
	if err == nil {
		nextNum = latest.RevisionNumber + 1
	}

	now := time.Now().UTC()
	hash := configHash(cfg)
	metaRaw := json.RawMessage("{}")
	if cfg.Metadata != nil {
		metaRaw, _ = json.Marshal(cfg.Metadata)
	}

	rev := &store.DeploymentRevision{
		ID:                 uuid.NewString(),
		DeploymentID:       deploymentID,
		RevisionNumber:     nextNum,
		ImageRef:           cfg.ImageRef,
		ComposeManifestRef: cfg.ComposeManifestRef,
		GitCommitSHA:       cfg.GitCommitSHA,
		ConfigHash:         hash,
		Status:             string(store.RevisionStatusPending),
		Description:        cfg.Description,
		Metadata:           metaRaw,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.store.CreateDeploymentRevision(ctx, rev); err != nil {
		return nil, fmt.Errorf("create revision: %w", err)
	}

	return toRevision(*rev), nil
}

func (s *Service) ListRevisions(ctx context.Context, deploymentID string) ([]*Revision, error) {
	revs, err := s.store.ListDeploymentRevisions(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	result := make([]*Revision, 0, len(revs))
	for _, r := range revs {
		result = append(result, toRevision(r))
	}
	return result, nil
}

func (s *Service) GetRevision(ctx context.Context, revisionID string) (*Revision, error) {
	r, err := s.store.GetDeploymentRevision(ctx, revisionID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}
	return toRevision(r), nil
}

func (s *Service) RollbackToRevision(ctx context.Context, deploymentID string, revisionID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	targetRev, err := s.store.GetDeploymentRevision(ctx, revisionID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}

	if targetRev.DeploymentID != deploymentID {
		return nil, fmt.Errorf("revision does not belong to this deployment")
	}

	deployment := toServiceDeployment(sd)
	deployment.Image = targetRev.ImageRef
	deployment.Status = StatusInProgress
	deployment.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("update deployment for rollback: %w", err)
	}

	if err := s.store.UpdateDeploymentCurrentRevision(ctx, deploymentID, &revisionID); err != nil {
		return nil, fmt.Errorf("update current revision: %w", err)
	}

	if err := s.store.SupersedeDeploymentRevisions(ctx, deploymentID, revisionID); err != nil {
		return nil, fmt.Errorf("supersede revisions: %w", err)
	}

	now := time.Now().UTC()
	if err := s.store.UpdateDeploymentRevisionStatus(ctx, revisionID, string(store.RevisionStatusActive), &now); err != nil {
		return nil, fmt.Errorf("activate revision: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_rolled_back", "deployment", "deployment", deploymentID, map[string]any{
			"serverId":     deployment.ServerID,
			"targetRev":    targetRev.RevisionNumber,
			"targetImage":  targetRev.ImageRef,
		}))
	}

	return deployment, nil
}

func (s *Service) RollbackToPrevious(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	if sd.CurrentRevisionID == nil {
		return nil, ErrNoRevisions
	}

	currentRev, err := s.store.GetDeploymentRevision(ctx, *sd.CurrentRevisionID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}

	prevRev, err := s.store.GetPreviousDeploymentRevision(ctx, deploymentID, currentRev.RevisionNumber)
	if err != nil {
		return nil, ErrNoRevisions
	}

	return s.RollbackToRevision(ctx, deploymentID, prevRev.ID)
}

func (s *Service) CompareRevisions(ctx context.Context, fromID string, toID string) (*RevisionDiff, error) {
	fromRev, err := s.store.GetDeploymentRevision(ctx, fromID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}

	toRev, err := s.store.GetDeploymentRevision(ctx, toID)
	if err != nil {
		return nil, ErrRevisionNotFound
	}

	var changes []RevisionChange

	if fromRev.ImageRef != toRev.ImageRef {
		changes = append(changes, RevisionChange{
			Field: "imageRef", OldValue: fromRev.ImageRef, NewValue: toRev.ImageRef,
		})
	}
	if fromRev.ComposeManifestRef != toRev.ComposeManifestRef {
		changes = append(changes, RevisionChange{
			Field: "composeManifestRef", OldValue: fromRev.ComposeManifestRef, NewValue: toRev.ComposeManifestRef,
		})
	}
	if fromRev.GitCommitSHA != toRev.GitCommitSHA {
		changes = append(changes, RevisionChange{
			Field: "gitCommitSha", OldValue: fromRev.GitCommitSHA, NewValue: toRev.GitCommitSHA,
		})
	}
	if fromRev.ConfigHash != toRev.ConfigHash {
		changes = append(changes, RevisionChange{
			Field: "configHash", OldValue: fromRev.ConfigHash, NewValue: toRev.ConfigHash,
		})
	}
	if string(fromRev.Metadata) != string(toRev.Metadata) {
		changes = append(changes, RevisionChange{
			Field: "metadata", OldValue: string(fromRev.Metadata), NewValue: string(toRev.Metadata),
		})
	}

	return &RevisionDiff{
		FromRevisionID: fromRev.RevisionNumber,
		ToRevisionID:   toRev.RevisionNumber,
		Changes:        changes,
	}, nil
}
