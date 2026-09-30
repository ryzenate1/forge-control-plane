package previewenv

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"strings"

	"gamepanel/forge/internal/store"
)

// This file wires provider PR/push webhook payloads into the preview
// lifecycle. It mirrors the verifier usage of handlers_git.go (the old
// push-only hooks stay untouched; these are the "preview" webhooks registered
// by phase4_registrar.go on /api/v1/preview/webhook/*).
//
// Signature verification reuses the exported git.Verify* helpers against the
// webhook secret of the matching auto-deploy git source. Write actions:
//   - pull_request opened/reopened/synchronize  -> upsert + deploy preview
//   - pull_request closed (merged or not)       -> cleanup
//   - push on a non-default branch              -> upsert per commit, destroy
//     when branch deletion (zero SHA)
//   - push on the default branch                -> ignored
//
// The GitSource lookup is shared with the push hooks, so previews trail the
// same auto-deploy git sources the GitOps pipeline already knows.

// resolveSource finds the auto-deploy git source for repoURL+branch and
// verifies the provider signature. Returns (nil, nil) when nothing matches or
// verification fails (mirrors the 200-OK drop behavior of handlers_git.go).
func (s *Service) resolveSource(ctx context.Context, repoURL, branch string, verify func(secret string, payload []byte) error, payload []byte) (*store.GitSource, error) {
	if repoURL == "" || branch == "" {
		return nil, nil
	}
	source, err := s.store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
	if err != nil || source == nil {
		return nil, nil
	}
	if source.WebhookSecret == "" {
		return nil, nil
	}
	if err := verify(source.WebhookSecret, payload); err != nil {
		if s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: webhook signature verification failed",
				"repo", repoURL, "branch", branch, "err", err)
		}
		return nil, nil
	}
	return source, nil
}

// upsertAndDeploy runs the one-active-preview-per-PR flow: if a live row
// exists it is refreshed (commit sha/title) and redeployed when needed;
// otherwise Create (ErrAlreadyExists collapses races) then Deploy.
func (s *Service) upsertAndDeploy(ctx context.Context, source *store.GitSource, req *store.PreviewDeployment) error {
	if req.CommitSHA == "" {
		return nil
	}

	actives, err := s.store.ListActivePreviewDeployments(ctx)
	if err != nil {
		return err
	}
	for i := range actives {
		p := &actives[i]
		if !samePreviewUnit(p, req) {
			continue
		}
		if p.CommitSHA == req.CommitSHA {
			if p.Status == "deploying" {
				return s.Deploy(ctx, p.ID)
			}
			return nil
		}
		if err := s.store.UpdatePreviewDeployment(ctx, &store.PreviewDeployment{
			ID:        p.ID,
			Status:    p.Status,
			CommitSHA: req.CommitSHA,
			PRTitle:   req.PRTitle,
			PRURL:     req.PRURL,
			PreviewURL: s.opts.PreviewURL(p.PRNumber, p.RepoOwner, p.RepoName),
		}); err != nil {
			return err
		}
		if p.Status == "deploying" {
			return s.Deploy(ctx, p.ID)
		}
		return nil
	}

	serverID, err := s.resolveServerID(ctx, source)
	if err != nil {
		if s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: no server linked to git source, preview skipped",
				"source", source.ID, "err", err)
		}
		return nil
	}
	created, err := s.Create(ctx, serverID, req)
	if err != nil {
		if err == ErrAlreadyExists {
			return nil
		}
		return err
	}
	return s.Deploy(ctx, created.ID)
}

// samePreviewUnit reports whether p and req are the same preview unit: the
// same PR number on the same repo, and (for branch-produced previews) the
// same branch.
func samePreviewUnit(p *store.PreviewDeployment, req *store.PreviewDeployment) bool {
	if p.PRNumber != req.PRNumber {
		return false
	}
	if !strings.EqualFold(p.RepoOwner, req.RepoOwner) || !strings.EqualFold(p.RepoName, req.RepoName) {
		return false
	}
	if req.Branch != "" && p.Branch != "" && p.Branch != req.Branch {
		return false
	}
	return true
}

// cleanupForPR destroys every live preview of the PR on the source repo
// (pull_request.closed, including a merged branch being deleted).
func (s *Service) cleanupForPR(ctx context.Context, owner, repo string, prNumber int) error {
	actives, err := s.store.ListActivePreviewDeployments(ctx)
	if err != nil {
		return err
	}
	for i := range actives {
		p := &actives[i]
		if p.PRNumber != prNumber || !strings.EqualFold(p.RepoOwner, owner) || !strings.EqualFold(p.RepoName, repo) {
			continue
		}
		if err := s.Cleanup(ctx, p.ID); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: cleanup on PR close failed", "preview", p.ID, "err", err)
		}
	}
	return nil
}

// cleanupForBranch destroys live previews of a branch on the source repo
// (branch deletion push, GitLab push with after=0000).
func (s *Service) cleanupForBranch(ctx context.Context, owner, repo, branch string) error {
	actives, err := s.store.ListActivePreviewDeployments(ctx)
	if err != nil {
		return err
	}
	for i := range actives {
		p := &actives[i]
		if p.Branch != branch || !strings.EqualFold(p.RepoOwner, owner) || !strings.EqualFold(p.RepoName, repo) {
			continue
		}
		if err := s.Cleanup(ctx, p.ID); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: branch cleanup failed", "preview", p.ID, "err", err)
		}
	}
	return nil
}

// --- payload parsing helpers (shared by the four handlers) ---

func isZeroSHA(sha string) bool {
	return strings.Trim(sha, "0") == ""
}

// prNumberForBranch derives a stable pseudo-PR number from branch+sha so
// plain pushes get their own preview row per push.
func prNumberForBranch(branch, sha string) int {
	h := fnv.New32a()
	h.Write([]byte(branch))
	h.Write([]byte{0})
	h.Write([]byte(sha))
	return int(h.Sum32()%1000000) + 1
}

func previewTitle(branch string) string { return "Preview: " + branch }

type cloneLink struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

func cloneURLFrom(links []cloneLink) string {
	for _, l := range links {
		if l.Name == "https" {
			return l.Href
		}
	}
	for _, l := range links {
		if l.Href != "" {
			return l.Href
		}
	}
	return ""
}

func splitFullName(full string) (owner, repo string) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return full, ""
}

type repositoryLinks struct {
	Clone []cloneLink `json:"clone"`
}

// findDefaultBranch sniffs default_branch hints from provider payloads (used
// to ignore default-branch pushes).
func findDefaultBranch(payload []byte) string {
	var probe struct {
		Repository struct {
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
		Project struct {
			DefaultBranch string `json:"default_branch"`
		} `json:"project"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return ""
	}
	if probe.Repository.DefaultBranch != "" {
		return probe.Repository.DefaultBranch
	}
	return probe.Project.DefaultBranch
}