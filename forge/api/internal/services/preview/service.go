package preview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type Service struct {
	store     *store.Store
	publisher events.Publisher
}

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:     store,
		publisher: publisher,
	}
}

var (
	ErrNotFound      = errors.New("preview deployment not found")
	ErrNotDeploying  = errors.New("preview deployment is not in deploying status")
	ErrAlreadyExists = errors.New("active preview deployment already exists for this PR")
)

func toServicePreview(p store.PreviewDeployment) *store.PreviewDeployment {
	return &p
}

func (s *Service) Create(ctx context.Context, serverID string, req *store.PreviewDeployment) (*store.PreviewDeployment, error) {
	now := time.Now().UTC()
	suffix := uuid.NewString()[:8]

	preview := &store.PreviewDeployment{
		ID:            uuid.NewString(),
		ServerID:      serverID,
		ServiceID:     req.ServiceID,
		PRNumber:      req.PRNumber,
		PRTitle:       req.PRTitle,
		PRURL:         req.PRURL,
		Branch:        req.Branch,
		RepoOwner:     req.RepoOwner,
		RepoName:      req.RepoName,
		CommitSHA:     req.CommitSHA,
		Status:        "deploying",
		Source:        req.Source,
		UniqueSuffix:  suffix,
		IsIsolated:    true,
		CreatedBy:     req.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.CreatePreviewDeployment(ctx, preview); err != nil {
		return nil, fmt.Errorf("create preview deployment: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("preview_deployment_created", "preview", "server", serverID, map[string]any{
			"previewId": preview.ID,
			"prNumber":  req.PRNumber,
			"branch":    req.Branch,
		}))
	}

	return preview, nil
}

func (s *Service) Get(ctx context.Context, id string) (*store.PreviewDeployment, error) {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *Service) List(ctx context.Context, serverID string) ([]store.PreviewDeployment, error) {
	return s.store.ListPreviewDeployments(ctx, serverID)
}

func (s *Service) ListAll(ctx context.Context) ([]store.PreviewDeployment, error) {
	return s.store.ListAllPreviewDeployments(ctx)
}

func (s *Service) UpdateStatus(ctx context.Context, id string, status string) error {
	if err := s.store.UpdatePreviewDeploymentStatus(ctx, id, status); err != nil {
		return fmt.Errorf("update preview status: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("preview_deployment_status_changed", "preview", "preview", id, map[string]any{
			"status": status,
		}))
	}

	return nil
}

func (s *Service) Deploy(ctx context.Context, id string) error {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	if p.Status != "deploying" {
		return ErrNotDeploying
	}

	if err := s.store.UpdatePreviewDeploymentStatus(ctx, id, "running"); err != nil {
		return fmt.Errorf("update preview status: %w", err)
	}

	previewURL := fmt.Sprintf("https://preview-%s.example.com", p.UniqueSuffix)
	pUpdated := &store.PreviewDeployment{
		ID:            id,
		Status:        "running",
		PreviewURL:    previewURL,
		DeploymentURL: fmt.Sprintf("https://deploy-%s.example.com", p.UniqueSuffix),
	}
	if err := s.store.UpdatePreviewDeployment(ctx, pUpdated); err != nil {
		return fmt.Errorf("update preview deployment: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("preview_deployment_running", "preview", "preview", id, map[string]any{
			"previewUrl": previewURL,
			"prNumber":   p.PRNumber,
		}))
	}

	return nil
}

func (s *Service) Cleanup(ctx context.Context, id string) error {
	p, err := s.store.GetPreviewDeployment(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	if p.Status == "cleaned_up" {
		return nil
	}

	if err := s.store.UpdatePreviewDeploymentStatus(ctx, id, "cleaned_up"); err != nil {
		return fmt.Errorf("cleanup preview: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("preview_deployment_cleaned_up", "preview", "preview", id, map[string]any{
			"prNumber": p.PRNumber,
		}))
	}

	return nil
}

func (s *Service) HandleWebhook(ctx context.Context, eventType string, payload map[string]any) error {
	switch eventType {
	case "pull_request.opened", "pull_request.synchronize":
		prNumber, _ := payload["pr_number"].(int)
		branch, _ := payload["branch"].(string)
		repoOwner, _ := payload["repo_owner"].(string)
		repoName, _ := payload["repo_name"].(string)
		commitSHA, _ := payload["commit_sha"].(string)
		serverID, _ := payload["server_id"].(string)

		if serverID == "" || prNumber == 0 {
			return errors.New("server_id and pr_number are required")
		}

		req := &store.PreviewDeployment{
			PRNumber:  prNumber,
			Branch:    branch,
			RepoOwner: repoOwner,
			RepoName:  repoName,
			CommitSHA: commitSHA,
			Source:    "github",
			PRTitle:   fmt.Sprintf("PR #%d", prNumber),
		}

		created, err := s.Create(ctx, serverID, req)
		if err != nil {
			return err
		}

		return s.Deploy(ctx, created.ID)

	case "pull_request.closed":
		prNumber, _ := payload["pr_number"].(int)
		serverID, _ := payload["server_id"].(string)

		previews, err := s.store.ListPreviewDeployments(ctx, serverID)
		if err != nil {
			return err
		}

		for _, p := range previews {
			if p.PRNumber == prNumber && p.Status != "cleaned_up" {
				if err := s.Cleanup(ctx, p.ID); err != nil {
					slog.Error("cleanup preview on PR close", "previewId", p.ID, "error", err)
				}
			}
		}

	default:
		slog.Debug("unhandled webhook event", "eventType", eventType)
	}

	return nil
}

func (s *Service) GetActivePreviews(ctx context.Context) ([]store.PreviewDeployment, error) {
	return s.store.ListActivePreviewDeployments(ctx)
}
