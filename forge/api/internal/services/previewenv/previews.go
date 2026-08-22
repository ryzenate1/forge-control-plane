package previewenv

// Project-scoped ephemeral preview deployments.
//
// A preview is a short-lived, branch-addressable deployment with its own
// subdomain (https://<branch>-<project-slug>.<baseDomain>). It is created
// either manually (POST /projects/:id/previews) or from a provider webhook
// (see preview_webhook.go), runs as its own compose stack (see
// preview_deploy.go) and is destroyed when the PR closes, when an operator
// deletes it, or when its TTL elapses.
//
// This file owns the model and the lifecycle entry points; it deliberately
// sits next to — not on top of — the legacy preview_deployments surface in
// service.go. See migrations/221_a_preview_environments.sql for why.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Preview lifecycle statuses. These mirror the CHECK constraint in
// preview_environments; anything outside this set is a bug, not a state.
const (
	PreviewStatusPending   = "pending"
	PreviewStatusDeploying = "deploying"
	PreviewStatusActive    = "active"
	PreviewStatusExpired   = "expired"
	PreviewStatusFailed    = "failed"
	PreviewStatusTeardown  = "teardown"
)

// maxSlugLen is the DNS label limit; a longer label is not "truncated
// gracefully", it is simply unresolvable.
const maxSlugLen = 63

var (
	// ErrPreviewNotFound is returned for unknown or already-deleted previews.
	ErrPreviewNotFound = errors.New("preview deployment not found")
	// ErrProjectNotFound is returned when the project id does not exist.
	ErrProjectNotFound = errors.New("preview project not found")
	// ErrServerNotFound is returned when a preview is pinned to an unknown server.
	ErrServerNotFound = errors.New("server not found")
	// ErrPreviewAlreadyExists is returned when the project already holds a live
	// preview for the same branch.
	ErrPreviewAlreadyExists = errors.New("a live preview already exists for this branch")
	// ErrPreviewTornDown is returned when a closed preview is asked to deploy
	// again: the hostname is gone, so a new preview has to be created.
	ErrPreviewTornDown = errors.New("preview was torn down; create a new preview instead")
	// ErrPreviewLimitReached is returned when a project is at its preview cap.
	ErrPreviewLimitReached = errors.New("preview limit reached for this project")
	// ErrNoComposeSource means nothing in the project can be previewed.
	ErrNoComposeSource = errors.New("project has no compose stack to preview; supply composeContent or baseStackId")
	// ErrAmbiguousComposeSource means more than one stack could be previewed
	// and the caller has to name one. Never resolved implicitly.
	ErrAmbiguousComposeSource = errors.New("project has multiple compose stacks; specify baseStackId or composeContent")
	// ErrComposeSourceOutsideProject is returned when baseStackId names a stack
	// that belongs to a different project.
	ErrComposeSourceOutsideProject = errors.New("the requested compose stack does not belong to this project")
	// ErrPreviewRuntimeUnavailable is returned when the service was built
	// without the persistence or deployment capability previews need. A
	// preview that cannot possibly run is never reported as created.
	ErrPreviewRuntimeUnavailable = errors.New("preview deployments are not configured on this control plane")
)

// PreviewDeployment is one project-scoped ephemeral deployment.
type PreviewDeployment struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"projectId"`
	EnvironmentID  *string    `json:"environmentId,omitempty"`
	ServerID       *string    `json:"serverId,omitempty"`
	Branch         string     `json:"branch"`
	PRNumber       int        `json:"prNumber,omitempty"`
	CommitSHA      string     `json:"commitSha,omitempty"`
	Slug           string     `json:"slug"`
	Status         string     `json:"status"`
	URL            string     `json:"url,omitempty"`
	ComposeContent string     `json:"composeContent,omitempty"`
	StackID        string     `json:"stackId,omitempty"`
	Source         string     `json:"source"`
	Title          string     `json:"title,omitempty"`
	PRURL          string     `json:"prUrl,omitempty"`
	CloseReason    string     `json:"closeReason,omitempty"`
	Error          string     `json:"error,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	CreatedBy      *string    `json:"createdBy,omitempty"`
}

// CreatePreviewRequest is the input for a manual or webhook-driven preview
// creation. Branch is the identity of the preview: it drives the slug and the
// one-live-preview-per-project rule.
type CreatePreviewRequest struct {
	ProjectID      string
	EnvironmentID  *string
	ServerID       *string
	Branch         string
	PRNumber       int
	CommitSHA      string
	Title          string
	PRURL          string
	ComposeContent string
	// BaseStackID names the compose stack to clone when ComposeContent is
	// empty. Left empty, the source is resolved from the project and only
	// deployed when exactly one candidate exists.
	BaseStackID string
	Source      string
	CreatedBy   string
	// TTL overrides the configured default for this preview only. A
	// non-positive value falls back to the default.
	TTL time.Duration
}

// IsLivePreviewStatus reports whether a status still occupies a branch slot
// and a concurrency permit.
func IsLivePreviewStatus(status string) bool {
	switch status {
	case PreviewStatusPending, PreviewStatusDeploying, PreviewStatusActive:
		return true
	default:
		return false
	}
}

// previews is the shared guard for the project-scoped surface: it fails loudly
// when the registrar built the service without a pool or a deployer instead of
// pretending a preview exists.
func (s *Service) previews() (PreviewEnvStore, error) {
	if s == nil || s.previewsStore == nil || s.deployer == nil {
		return nil, ErrPreviewRuntimeUnavailable
	}
	return s.previewsStore, nil
}

// CreatePreview provisions an ephemeral preview for a branch or PR.
//
// The returned row carries status "pending": provisioning runs asynchronously
// against the compose lifecycle and lands in "active" or "failed", which the
// list/get endpoints and the event stream observe. A create that cannot even
// resolve a compose source, or a control plane without a deployer, returns an
// error and persists nothing.
func (s *Service) CreatePreview(ctx context.Context, req CreatePreviewRequest) (*PreviewDeployment, error) {
	repo, err := s.previews()
	if err != nil {
		return nil, err
	}

	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Branch = strings.TrimSpace(req.Branch)
	if req.ProjectID == "" {
		return nil, fmt.Errorf("projectId is required")
	}
	if req.Branch == "" {
		return nil, fmt.Errorf("branch is required")
	}
	if req.Source == "" {
		req.Source = "manual"
	}

	project, err := repo.GetPreviewProject(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	// Concurrency and identity are both enforced here rather than only in
	// memory: the per-branch partial unique index closes the race between two
	// concurrent webhooks for the same branch.
	live, err := repo.FindLivePreviewEnvByBranch(ctx, req.ProjectID, req.Branch)
	if err != nil && !errors.Is(err, ErrPreviewNotFound) {
		return nil, err
	}
	if live != nil {
		return nil, ErrPreviewAlreadyExists
	}
	if s.opts.MaxPreviewsPerProject > 0 {
		count, err := repo.CountLivePreviewEnvs(ctx, req.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("count live previews: %w", err)
		}
		if count >= s.opts.MaxPreviewsPerProject {
			return nil, fmt.Errorf("%w: %d live previews", ErrPreviewLimitReached, count)
		}
	}

	source, err := s.resolveComposeSource(ctx, repo, req)
	if err != nil {
		return nil, err
	}

	nodeID, err := s.resolvePreviewNode(ctx, repo, req.ServerID, source)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = source.UserID
	}
	envID := req.EnvironmentID
	if (envID == nil || *envID == "") && source.EnvironmentID != "" {
		envID = &source.EnvironmentID
	}

	ttl := s.opts.PreviewTTL
	if req.TTL > 0 {
		ttl = req.TTL
	}
	var expiresAt *time.Time
	if ttl > 0 {
		expiry := now.Add(ttl)
		expiresAt = &expiry
	}

	preview := &PreviewDeployment{
		ProjectID:      req.ProjectID,
		EnvironmentID:  envID,
		ServerID:       req.ServerID,
		Branch:         req.Branch,
		PRNumber:       req.PRNumber,
		CommitSHA:      req.CommitSHA,
		Status:         PreviewStatusPending,
		ComposeContent: source.ComposeYAML,
		Source:         req.Source,
		Title:          req.Title,
		PRURL:          req.PRURL,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      nilIfEmpty(createdBy),
	}

	// The slug is the subdomain, so it is allocated by attempting the insert:
	// the UNIQUE index — not a read-then-write scan — decides who wins.
	if err := s.createPreviewWithUniqueSlug(ctx, repo, preview, buildPreviewSlug(req.Branch, project.Slug)); err != nil {
		return nil, err
	}

	s.publish(ctx, "preview_environment_created", preview.ID, map[string]any{
		"previewId": preview.ID,
		"projectId": preview.ProjectID,
		"branch":    preview.Branch,
		"url":       preview.URL,
	})

	s.startProvision(preview.ID, nodeID, createdBy)
	return preview, nil
}

// ListPreviews returns every preview belonging to a project, newest first.
func (s *Service) ListPreviews(ctx context.Context, projectID string) ([]PreviewDeployment, error) {
	repo, err := s.previews()
	if err != nil {
		return nil, err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("projectId is required")
	}
	items, err := repo.ListPreviewEnvsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []PreviewDeployment{}
	}
	return items, nil
}

// GetPreview loads one preview by id.
func (s *Service) GetPreview(ctx context.Context, id string) (*PreviewDeployment, error) {
	repo, err := s.previews()
	if err != nil {
		return nil, err
	}
	return repo.GetPreviewEnv(ctx, strings.TrimSpace(id))
}

// GetPreviewInProject loads a preview and refuses to return it when it belongs
// to another project. Project-scoped handlers must use this instead of
// GetPreview: knowing a preview id is not authorization to read someone else's
// preview through a project you do have access to.
func (s *Service) GetPreviewInProject(ctx context.Context, projectID, id string) (*PreviewDeployment, error) {
	preview, err := s.GetPreview(ctx, id)
	if err != nil {
		return nil, err
	}
	if preview.ProjectID != strings.TrimSpace(projectID) {
		return nil, ErrPreviewNotFound
	}
	return preview, nil
}

// RedeployPreviewInProject is RedeployPreview gated on project ownership.
func (s *Service) RedeployPreviewInProject(ctx context.Context, projectID, id string) (*PreviewDeployment, error) {
	if _, err := s.GetPreviewInProject(ctx, projectID, id); err != nil {
		return nil, err
	}
	return s.RedeployPreview(ctx, id)
}

// ClosePreviewInProject is ClosePreview gated on project ownership.
func (s *Service) ClosePreviewInProject(ctx context.Context, projectID, id, reason string) error {
	if _, err := s.GetPreviewInProject(ctx, projectID, id); err != nil {
		return err
	}
	return s.ClosePreview(ctx, id, reason)
}

// DeletePreviewInProject is DeletePreview gated on project ownership.
func (s *Service) DeletePreviewInProject(ctx context.Context, projectID, id string) error {
	if _, err := s.GetPreviewInProject(ctx, projectID, id); err != nil {
		return err
	}
	return s.DeletePreview(ctx, id)
}

// RedeployPreview re-runs the provisioning step for an existing preview,
// updating it in place when its compose stack is still around.
func (s *Service) RedeployPreview(ctx context.Context, id string) (*PreviewDeployment, error) {
	repo, err := s.previews()
	if err != nil {
		return nil, err
	}
	preview, err := repo.GetPreviewEnv(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if preview.Status == PreviewStatusTeardown {
		return nil, fmt.Errorf("%w: %s", ErrPreviewTornDown, preview.ID)
	}

	nodeID := ""
	if preview.ServerID != nil && *preview.ServerID != "" {
		nodeID, err = repo.NodeIDForServer(ctx, *preview.ServerID)
		if err != nil {
			return nil, err
		}
	}

	preview.Status = PreviewStatusPending
	preview.Error = ""
	now := time.Now().UTC()
	preview.UpdatedAt = now
	if s.opts.PreviewTTL > 0 {
		expiry := now.Add(s.opts.PreviewTTL)
		preview.ExpiresAt = &expiry
	}
	if err := repo.UpdatePreviewEnv(ctx, preview); err != nil {
		return nil, fmt.Errorf("reset preview for redeploy: %w", err)
	}

	createdBy := ""
	if preview.CreatedBy != nil {
		createdBy = *preview.CreatedBy
	}
	s.startProvision(preview.ID, nodeID, createdBy)
	return preview, nil
}

// ClosePreview tears a preview down but keeps the row as an audit trail
// (status "teardown"). It is idempotent: closing a closed preview succeeds.
func (s *Service) ClosePreview(ctx context.Context, id string, reason string) error {
	repo, err := s.previews()
	if err != nil {
		return err
	}
	preview, err := repo.GetPreviewEnv(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if preview.Status == PreviewStatusTeardown || preview.Status == PreviewStatusExpired {
		return nil
	}

	if reason == "" {
		reason = "closed"
	}

	// Destroy the running workload first. If that fails we keep the row live
	// enough to retry, because a preview whose containers are still running
	// must never be reported as closed.
	if err := s.destroyPreviewStack(ctx, preview); err != nil {
		return err
	}

	preview.Status = PreviewStatusTeardown
	preview.CloseReason = reason
	preview.UpdatedAt = time.Now().UTC()
	if err := repo.UpdatePreviewEnv(ctx, preview); err != nil {
		return fmt.Errorf("record preview teardown: %w", err)
	}

	s.withdrawPreviewRoute(ctx, preview)
	s.publish(ctx, "preview_environment_closed", preview.ID, map[string]any{
		"previewId": preview.ID,
		"projectId": preview.ProjectID,
		"branch":    preview.Branch,
		"reason":    reason,
	})
	return nil
}

// DeletePreview tears the workload down and removes the row entirely.
func (s *Service) DeletePreview(ctx context.Context, id string) error {
	repo, err := s.previews()
	if err != nil {
		return err
	}
	preview, err := repo.GetPreviewEnv(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if err := s.destroyPreviewStack(ctx, preview); err != nil {
		return err
	}
	s.withdrawPreviewRoute(ctx, preview)
	if err := repo.DeletePreviewEnv(ctx, preview.ID); err != nil {
		return err
	}
	s.publish(ctx, "preview_environment_deleted", preview.ID, map[string]any{
		"previewId": preview.ID,
		"projectId": preview.ProjectID,
		"branch":    preview.Branch,
	})
	return nil
}

// CleanupExpired destroys every live preview whose TTL has elapsed and returns
// how many were reaped. Failures are collected, not swallowed: a partial pass
// reports the count it achieved together with what went wrong.
func (s *Service) CleanupExpired(ctx context.Context) (int, error) {
	repo, err := s.previews()
	if err != nil {
		return 0, err
	}
	expired, err := repo.ListExpiredPreviewEnvs(ctx, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("list expired previews: %w", err)
	}

	reaped := 0
	var errs []error
	for i := range expired {
		p := &expired[i]
		if err := s.expirePreview(ctx, repo, p); err != nil {
			errs = append(errs, fmt.Errorf("preview %s: %w", p.ID, err))
			continue
		}
		reaped++
	}
	return reaped, errors.Join(errs...)
}

// expirePreview marks the row expired and destroys its workload. The status is
// written before the stack is destroyed so a crash in between leaves a
// visibly-expired row rather than a row that claims to still be serving.
func (s *Service) expirePreview(ctx context.Context, repo PreviewEnvStore, p *PreviewDeployment) error {
	p.Status = PreviewStatusExpired
	p.CloseReason = "ttl-expired"
	p.UpdatedAt = time.Now().UTC()
	if err := repo.UpdatePreviewEnv(ctx, p); err != nil {
		return fmt.Errorf("mark expired: %w", err)
	}
	if err := s.destroyPreviewStack(ctx, p); err != nil {
		return err
	}
	s.withdrawPreviewRoute(ctx, p)
	s.publish(ctx, "preview_environment_expired", p.ID, map[string]any{
		"previewId": p.ID,
		"projectId": p.ProjectID,
		"branch":    p.Branch,
	})
	return nil
}

// ---- compose source / placement resolution ----

func (s *Service) resolveComposeSource(ctx context.Context, repo PreviewEnvStore, req CreatePreviewRequest) (PreviewComposeSource, error) {
	explicit := strings.TrimSpace(req.ComposeContent)
	if explicit != "" {
		envID := ""
		if req.EnvironmentID != nil {
			envID = *req.EnvironmentID
		}
		return PreviewComposeSource{ComposeYAML: explicit, EnvironmentID: envID, UserID: req.CreatedBy}, nil
	}
	source, err := repo.ResolveComposeSource(ctx, req.ProjectID, strings.TrimSpace(req.BaseStackID))
	if err != nil {
		return PreviewComposeSource{}, err
	}
	if strings.TrimSpace(source.ComposeYAML) == "" {
		return PreviewComposeSource{}, ErrNoComposeSource
	}
	return source, nil
}

// resolvePreviewNode decides which node runs the preview. A caller-pinned
// server wins; otherwise the base stack's own node is reused; otherwise the
// compose scheduler places it (empty node id).
func (s *Service) resolvePreviewNode(ctx context.Context, repo PreviewEnvStore, serverID *string, source PreviewComposeSource) (string, error) {
	if serverID != nil && *serverID != "" {
		nodeID, err := repo.NodeIDForServer(ctx, *serverID)
		if err != nil {
			return "", err
		}
		return nodeID, nil
	}
	return source.NodeID, nil
}

// ---- slug allocation ----

// createPreviewWithUniqueSlug inserts preview, retrying slug collisions with a
// short random suffix. A branch collision is never retried — it is a real
// conflict the caller has to see.
func (s *Service) createPreviewWithUniqueSlug(ctx context.Context, repo PreviewEnvStore, preview *PreviewDeployment, baseSlug string) error {
	slug := truncateSlug(baseSlug)
	for attempt := 0; attempt < 6; attempt++ {
		preview.ID = uuid.NewString()
		preview.Slug = slug
		preview.URL = s.previewURL(slug)
		err := repo.CreatePreviewEnv(ctx, preview)
		if err == nil {
			return nil
		}
		switch constraint := uniqueConstraint(err); {
		case constraint == "preview_environments_branch_unique":
			return ErrPreviewAlreadyExists
		case constraint == "preview_environments_slug_key":
			slug = slugWithSuffix(baseSlug)
			continue
		case isUniqueViolation(err):
			// Driver/test-double path: the constraint name is unavailable, so
			// fall back to matching the message. A branch conflict still
			// mentions the branch index, so it is checked first.
			if strings.Contains(err.Error(), "branch_unique") {
				return ErrPreviewAlreadyExists
			}
			slug = slugWithSuffix(baseSlug)
			continue
		default:
			return fmt.Errorf("create preview: %w", err)
		}
	}
	return fmt.Errorf("could not allocate a unique preview hostname for %q", baseSlug)
}

// buildPreviewSlug derives the subdomain label from "{branch}-{project-slug}",
// sanitized to DNS-label characters and capped at 63 characters.
func buildPreviewSlug(branch, projectSlug string) string {
	label := sanitizeSlugPart(branch)
	if project := sanitizeSlugPart(projectSlug); project != "" && project != "preview" {
		label = label + "-" + project
	}
	if label == "" {
		label = "preview"
	}
	// Keep the branch readable: when the combined label is too long, the tail
	// (the project part) is what gets cut, not the branch.
	if len(label) > maxSlugLen {
		label = strings.Trim(label[:maxSlugLen], "-")
	}
	return label
}

func slugWithSuffix(base string) string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand is expected to never fail; a uuid suffix is still unique
		// and still a legal DNS label.
		return truncateSlug(base + "-" + uuid.NewString()[:5])
	}
	return truncateSlug(base + "-" + hex.EncodeToString(buf[:]))
}

func truncateSlug(s string) string {
	if len(s) > maxSlugLen {
		s = s[:maxSlugLen]
	}
	return strings.Trim(s, "-")
}

// sanitizeSlugPart lowercases and reduces a string to [a-z0-9-], which is all
// a DNS label can carry. Underscores and dots become separators rather than
// being dropped, so "feature/foo_bar" stays readable as "feature-foo-bar".
func sanitizeSlugPart(s string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if pendingSep && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingSep = false
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == '/':
			pendingSep = true
		default:
			pendingSep = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// previewURL is the full external address of a preview. An empty configured
// domain would produce an unusable hostname, so it resolves to the documented
// default rather than to a bare "." suffix.
func (s *Service) previewURL(slug string) string {
	return "https://" + slug + "." + s.PreviewBaseDomain()
}

// PreviewBaseDomain exposes the domain previews are issued under.
func (s *Service) PreviewBaseDomain() string {
	if s != nil && s.opts.PreviewDomain != "" {
		return s.opts.PreviewDomain
	}
	return defaultPreviewDomain
}

// defaultPreviewDomain is the fallback when PREVIEW_DOMAIN is unset. It is a
// placeholder on purpose: routing still works off the recorded slug, and the
// operator sees an obviously-non-routable suffix in the UI until they
// configure the real wildcard domain.
const defaultPreviewDomain = "previews.forge.local"

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := s
	return &v
}
