package previewenv

// Provider webhooks → project previews.
//
// This is the second, project-scoped webhook path. It deliberately does not
// reuse the legacy HandleGitHubWebhook / HandleGitLabWebhookEvent flow in
// providers.go: that one resolves a GitSource and writes preview_deployments,
// which requires a repo→server mapping this feature has no use for. Here the
// project is named explicitly by the URL (and proven by that project's own
// signing secret), so a repository does not have to be wired to a Forge server
// for a preview to exist.
//
// Event mapping (Dokploy/CapRover style branch filtering):
//
//	github pull_request opened|reopened|synchronize|ready_for_review -> create/redeploy
//	github pull_request closed (merged or not)                        -> teardown
//	github push on a non-default branch                               -> create/redeploy
//	github push with deleted=true or a zero SHA                       -> teardown
//	gitlab merge_request open|reopen|update                           -> create/redeploy
//	gitlab merge_request close|merge                                  -> teardown
//	gitlab push                                                       -> as above
// anything else -> ErrPreviewEventIgnored, never a silent 200.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/services/git"
)

// Recognised preview webhook providers. Bitbucket and Gitea keep their existing
// legacy handlers (providers.go); this surface is GitHub + GitLab as briefed.
const (
	PreviewProviderGitHub = "github"
	PreviewProviderGitLab = "gitlab"
)

var (
	// ErrPreviewEventIgnored wraps every event that carries no preview intent.
	// Handlers map it to 202 Accepted so the provider stops retrying while the
	// response still says nothing was deployed.
	ErrPreviewEventIgnored = errors.New("preview webhook event ignored")
	// ErrPreviewWebhookNotConfigured is returned when a project has no signing
	// secret yet. Unsigned callbacks are rejected rather than trusted.
	ErrPreviewWebhookNotConfigured = errors.New("project has no preview webhook secret; generate one before receiving events")
	// ErrPreviewProjectRequired is returned when a callback did not name a
	// project, so there is no secret to verify it against.
	ErrPreviewProjectRequired = errors.New("preview webhooks must address a project")
)

// WebhookPayload is the provider-neutral intent extracted from a GitHub or
// GitLab callback. ProjectID is never taken from the payload: it comes from the
// route, which is what makes the per-project secret meaningful.
type WebhookPayload struct {
	Provider string `json:"provider"`
	// Event is the normalized kind: "pull_request", "merge_request" or "push".
	Event string `json:"event"`
	// Action is the provider's own verb, kept verbatim for audit output.
	Action        string `json:"action"`
	ProjectID     string `json:"projectId"`
	Branch        string `json:"branch"`
	PRNumber      int    `json:"prNumber,omitempty"`
	CommitSHA     string `json:"commitSha,omitempty"`
	Title         string `json:"title,omitempty"`
	PRURL         string `json:"prUrl,omitempty"`
	RepoOwner     string `json:"repoOwner,omitempty"`
	RepoName      string `json:"repoName,omitempty"`
	DefaultBranch string `json:"defaultBranch,omitempty"`
	// Deleted marks a branch-deletion push; Merged distinguishes a merged PR
	// close from a plain one so the teardown reason is accurate.
	Deleted bool `json:"deleted,omitempty"`
	Merged  bool `json:"merged,omitempty"`
	// ComposeContent / BaseStackID let a caller pin the deployable document to
	// an event; left empty the preview clones the project's own stack.
	ComposeContent string
	BaseStackID    string
	ServerID       string
	EnvironmentID  string
	CreatedBy      string
	TTL            time.Duration
}

// ParsePreviewWebhook normalizes a raw provider callback. eventHeader is the
// X-GitHub-Event value (GitHub only); GitLab self-describes in the body.
func ParsePreviewWebhook(body []byte, provider, eventHeader string) (WebhookPayload, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case PreviewProviderGitHub:
		return parseGitHubPreviewEvent(body, eventHeader)
	case PreviewProviderGitLab:
		return parseGitLabPreviewEvent(body)
	default:
		return WebhookPayload{}, fmt.Errorf("%w: unsupported preview webhook provider %q", ErrPreviewEventIgnored, provider)
	}
}

func parseGitHubPreviewEvent(body []byte, eventHeader string) (WebhookPayload, error) {
	switch strings.ToLower(strings.TrimSpace(eventHeader)) {
	case "pull_request":
		var ev githubEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			return WebhookPayload{}, fmt.Errorf("github pull_request payload: %w", err)
		}
		owner, repo := splitFullName(ev.Repository.FullName)
		if owner == "" {
			owner = ev.PR.Head.Repo.Owner.Login
		}
		if repo == "" {
			repo = ev.PR.Head.Repo.Name
		}
		if ev.PR.Head.Ref == "" || ev.Number == 0 {
			return WebhookPayload{}, fmt.Errorf("%w: pull_request carries no head branch or number", ErrPreviewEventIgnored)
		}
		return WebhookPayload{
			Provider:      PreviewProviderGitHub,
			Event:         "pull_request",
			Action:        ev.Action,
			Branch:        ev.PR.Head.Ref,
			PRNumber:      ev.Number,
			CommitSHA:     ev.PR.Head.SHA,
			Title:         ev.PR.Title,
			PRURL:         ev.PR.HTMLURL,
			RepoOwner:     owner,
			RepoName:      repo,
			Merged:        ev.PR.Merged,
			DefaultBranch: findDefaultBranch(body),
		}, nil
	case "push":
		var ev pushEventPayload
		if err := json.Unmarshal(body, &ev); err != nil {
			return WebhookPayload{}, fmt.Errorf("github push payload: %w", err)
		}
		owner, repo := splitFullName(ev.Repository.FullName)
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		if branch == "" || branch == "heads" {
			return WebhookPayload{}, fmt.Errorf("%w: push is not a branch ref", ErrPreviewEventIgnored)
		}
		// GitHub fills after with the pushed head, and sets it to all zeros on a
		// branch deletion. commits is the push's commit list, whose last entry is
		// the same head.
		sha := ev.After
		if sha == "" && len(ev.Commits) > 0 {
			sha = ev.Commits[len(ev.Commits)-1].ID
		}
		return WebhookPayload{
			Provider:      PreviewProviderGitHub,
			Event:         "push",
			Action:        "push",
			Branch:        branch,
			CommitSHA:     sha,
			RepoOwner:     owner,
			RepoName:      repo,
			Deleted:       ev.Deleted || isZeroSHA(sha),
			DefaultBranch: findDefaultBranch(body),
		}, nil
	default:
		return WebhookPayload{}, fmt.Errorf("%w: unhandled github event %q", ErrPreviewEventIgnored, eventHeader)
	}
}

func parseGitLabPreviewEvent(body []byte) (WebhookPayload, error) {
	var ev gitlabEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return WebhookPayload{}, fmt.Errorf("gitlab payload: %w", err)
	}
	namespace, repo := splitFullName(ev.Project.PathWithNS)

	switch ev.ObjectKind {
	case "merge_request":
		if ev.ObjectAttributes.SourceBranch == "" || ev.ObjectAttributes.IID == 0 {
			return WebhookPayload{}, fmt.Errorf("%w: merge_request carries no source branch or iid", ErrPreviewEventIgnored)
		}
		sha := ev.ObjectAttributes.SHA
		if sha == "" {
			sha = ev.ObjectAttributes.LastCommit.ID
		}
		return WebhookPayload{
			Provider:      PreviewProviderGitLab,
			Event:         "merge_request",
			Action:        ev.ObjectAttributes.Action,
			Branch:        ev.ObjectAttributes.SourceBranch,
			PRNumber:      ev.ObjectAttributes.IID,
			CommitSHA:     sha,
			Title:         ev.ObjectAttributes.Title,
			PRURL:         ev.ObjectAttributes.URL,
			RepoOwner:     namespace,
			RepoName:      repo,
			Merged:        strings.EqualFold(ev.ObjectAttributes.Action, "merge"),
			DefaultBranch: ev.Project.DefaultBranch,
		}, nil
	case "push":
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		if branch == "" {
			return WebhookPayload{}, fmt.Errorf("%w: push is not a branch ref", ErrPreviewEventIgnored)
		}
		sha := ev.After
		if sha == "" && len(ev.Commits) > 0 {
			sha = ev.Commits[len(ev.Commits)-1].ID
		}
		return WebhookPayload{
			Provider:      PreviewProviderGitLab,
			Event:         "push",
			Action:        "push",
			Branch:        branch,
			CommitSHA:     sha,
			RepoOwner:     namespace,
			RepoName:      repo,
			Deleted:       isZeroSHA(ev.After) && ev.After != "",
			DefaultBranch: ev.Project.DefaultBranch,
		}, nil
	default:
		return WebhookPayload{}, fmt.Errorf("%w: unhandled gitlab object_kind %q", ErrPreviewEventIgnored, ev.ObjectKind)
	}
}

// ---- verification ----

// VerifyPreviewWebhook checks a raw callback against the project's own signing
// secret. GitHub signs with HMAC-SHA256 (X-Hub-Signature-256); GitLab sends the
// secret verbatim as X-Gitlab-Token. A project without a generated secret has
// no way to authenticate a caller, so the request is refused rather than
// treated as unsigned-but-harmless.
func (s *Service) VerifyPreviewWebhook(ctx context.Context, projectID, provider string, body []byte, hubSignature, gitlabToken string) error {
	repo, err := s.previews()
	if err != nil {
		return err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ErrPreviewProjectRequired
	}
	project, err := repo.GetPreviewProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.WebhookSecret == "" {
		return ErrPreviewWebhookNotConfigured
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case PreviewProviderGitHub:
		return git.VerifyGitHubSignature(body, strings.TrimSpace(hubSignature), project.WebhookSecret)
	case PreviewProviderGitLab:
		return git.VerifyGitLabSignature(body, gitlabToken, project.WebhookSecret)
	default:
		return fmt.Errorf("unknown preview webhook provider %q", provider)
	}
}

// PreviewWebhookSecret returns the project's signing secret, generating and
// persisting one when it has none. rotate discards the existing secret, which
// immediately invalidates every callback already in flight for that project.
func (s *Service) PreviewWebhookSecret(ctx context.Context, projectID string, rotate bool) (string, error) {
	repo, err := s.previews()
	if err != nil {
		return "", err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", ErrPreviewProjectRequired
	}
	project, err := repo.GetPreviewProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	if project.WebhookSecret != "" && !rotate {
		return project.WebhookSecret, nil
	}

	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate preview webhook secret: %w", err)
	}
	secret := hex.EncodeToString(buf[:])
	if err := repo.SetProjectPreviewSecret(ctx, projectID, secret); err != nil {
		return "", fmt.Errorf("store preview webhook secret: %w", err)
	}
	return secret, nil
}

// WebhookURL is the address a provider should post to for projectID.
func (s *Service) WebhookURL(projectID string) string {
	if s == nil {
		return "/api/v1/webhooks/preview"
	}
	base := strings.TrimRight(s.opts.PanelURL, "/")
	if base == "" {
		return "/api/v1/webhooks/preview/" + projectID
	}
	return base + "/api/v1/webhooks/preview/" + projectID
}

// ---- trigger ----

// TriggerFromWebhook applies one normalized event to the preview lifecycle.
//
// The returned preview is the row the event now owns: freshly created, or the
// live row whose commit was recorded. A create is only *pending* here — the
// deployment itself is asynchronous — so callers must read the status rather
// than infer success from a nil error. Events that carry no preview intent
// return ErrPreviewEventIgnored.
func (s *Service) TriggerFromWebhook(ctx context.Context, payload WebhookPayload) (*PreviewDeployment, error) {
	repo, err := s.previews()
	if err != nil {
		return nil, err
	}
	payload.ProjectID = strings.TrimSpace(payload.ProjectID)
	if payload.ProjectID == "" {
		return nil, ErrPreviewProjectRequired
	}
	payload.Branch = strings.TrimSpace(payload.Branch)
	if payload.Branch == "" {
		return nil, fmt.Errorf("branch is required")
	}
	if _, err := repo.GetPreviewProject(ctx, payload.ProjectID); err != nil {
		return nil, err
	}

	live, err := s.livePreviewForEvent(ctx, repo, payload)
	if err != nil {
		return nil, err
	}

	if previewEventCloses(payload) {
		if live == nil {
			return nil, fmt.Errorf("%w: no live preview for branch %q to tear down", ErrPreviewEventIgnored, payload.Branch)
		}
		reason := previewCloseReason(payload)
		if err := s.ClosePreview(ctx, live.ID, reason); err != nil {
			return nil, err
		}
		live.Status = PreviewStatusTeardown
		live.CloseReason = reason
		return live, nil
	}

	if !previewEventDeploys(payload) {
		return nil, fmt.Errorf("%w: %s %s does not affect previews", ErrPreviewEventIgnored, payload.Provider, payload.Action)
	}
	if payload.Event == "push" && payload.DefaultBranch != "" && strings.EqualFold(payload.Branch, payload.DefaultBranch) {
		return nil, fmt.Errorf("%w: %s is the default branch", ErrPreviewEventIgnored, payload.Branch)
	}
	if strings.TrimSpace(payload.CommitSHA) == "" {
		return nil, fmt.Errorf("event for branch %q carries no commit; cannot identify the build", payload.Branch)
	}

	// The PR head moved to a different branch: the hostname is derived from the
	// branch, so the old preview is retired and a new one takes its place.
	if live != nil && !strings.EqualFold(live.Branch, payload.Branch) {
		if err := s.ClosePreview(ctx, live.ID, "branch renamed"); err != nil {
			return nil, err
		}
		live = nil
	}

	if live == nil {
		req := CreatePreviewRequest{
			ProjectID:      payload.ProjectID,
			Branch:         payload.Branch,
			PRNumber:       payload.PRNumber,
			CommitSHA:      payload.CommitSHA,
			Title:          payload.Title,
			PRURL:          payload.PRURL,
			ComposeContent: payload.ComposeContent,
			BaseStackID:    payload.BaseStackID,
			ServerID:       nilIfEmpty(payload.ServerID),
			EnvironmentID:  nilIfEmpty(payload.EnvironmentID),
			Source:         payload.Provider,
			CreatedBy:      payload.CreatedBy,
			TTL:            payload.TTL,
		}
		return s.CreatePreview(ctx, req)
	}

	// A provision already owns this row. Starting a second one would race the
	// first over the same status transitions, so the in-flight deploy wins and
	// the callback is acknowledged with the current state.
	if live.Status == PreviewStatusPending || live.Status == PreviewStatusDeploying {
		return live, nil
	}
	if strings.EqualFold(live.CommitSHA, payload.CommitSHA) {
		// Same commit already built: nothing changed, so nothing is redeployed.
		return live, nil
	}

	live.CommitSHA = payload.CommitSHA
	if payload.Title != "" {
		live.Title = payload.Title
	}
	if payload.PRURL != "" {
		live.PRURL = payload.PRURL
	}
	if payload.PRNumber > 0 {
		live.PRNumber = payload.PRNumber
	}
	live.UpdatedAt = time.Now().UTC()
	if err := repo.UpdatePreviewEnv(ctx, live); err != nil {
		return nil, fmt.Errorf("record webhook commit: %w", err)
	}
	return s.RedeployPreview(ctx, live.ID)
}

// livePreviewForEvent resolves the row an event refers to: the branch is the
// identity, with the PR number as a fallback so a renamed head branch still
// finds its preview.
func (s *Service) livePreviewForEvent(ctx context.Context, repo PreviewEnvStore, payload WebhookPayload) (*PreviewDeployment, error) {
	live, err := repo.FindLivePreviewEnvByBranch(ctx, payload.ProjectID, payload.Branch)
	if err != nil && !errors.Is(err, ErrPreviewNotFound) {
		return nil, err
	}
	if live != nil {
		return live, nil
	}
	if payload.PRNumber <= 0 {
		return nil, nil
	}
	byPR, err := repo.FindLivePreviewEnvByPR(ctx, payload.ProjectID, payload.PRNumber)
	if err != nil {
		if errors.Is(err, ErrPreviewNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return byPR, nil
}

func previewEventCloses(p WebhookPayload) bool {
	switch p.Event {
	case "pull_request":
		return p.Action == "closed"
	case "merge_request":
		return p.Action == "close" || p.Action == "closed" || p.Action == "merge"
	case "push":
		return p.Deleted
	default:
		return false
	}
}

func previewEventDeploys(p WebhookPayload) bool {
	switch p.Event {
	case "pull_request":
		switch p.Action {
		case "opened", "reopened", "synchronize", "ready_for_review":
			return true
		}
	case "merge_request":
		switch p.Action {
		case "open", "reopen", "update":
			return true
		}
	case "push":
		return true
	}
	return false
}

func previewCloseReason(p WebhookPayload) string {
	switch {
	case p.Event == "push":
		return "branch-deleted"
	case p.Merged:
		return "pr-merged"
	default:
		return "pr-closed"
	}
}
