package previewenv

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/store"
)

// Provider webhook handlers. Each handler is registered on a public route by
// internal/http/phase4_registrar.go and reuses the header semantics of the
// matching push-only hook: GitHub X-Hub-Signature-256, GitLab X-Gitlab-Token,
// Bitbucket X-Hub-Signature, Gitea X-Gitea-Signature.

type sigVerifier func(secret string, payload []byte) error

func verifyGithub(header string) sigVerifier {
	return func(secret string, payload []byte) error { return git.VerifyGitHubSignature(payload, header, secret) }
}
func verifyGitLab(header string) sigVerifier {
	return func(secret string, payload []byte) error { return git.VerifyGitLabSignature(payload, header, secret) }
}
func verifyBitbucket(header string) sigVerifier {
	return func(secret string, payload []byte) error { return git.VerifyBitbucketSignature(payload, header, secret) }
}
func verifyGitea(header string) sigVerifier {
	return func(secret string, payload []byte) error { return git.VerifyGiteaSignature(payload, header, secret) }
}

// ---- GitHub ----

type githubEvent struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	PR     struct {
		Title       string `json:"title"`
		HTMLURL     string `json:"html_url"`
		Merged      bool   `json:"merged"`
		Head        struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				Owner struct {
					Login string `json:"login"`
				} `json:"owner"`
				Name string `json:"name"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
}

// HandleGitHubWebhook processes X-GitHub-Event payloads ("pull_request" and
// "push"); signature is the X-Hub-Signature-256 header value.
func (s *Service) HandleGitHubWebhook(ctx context.Context, payload []byte, event, signature string) error {
	switch event {
	case "pull_request":
		var ev githubEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("github pull_request payload: %w", err)
		}
		return s.handleGitHubPR(ctx, ev, payload, signature)
	case "push":
		var ev pushEventPayload
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("github push payload: %w", err)
		}
		return s.handleBranchPush(ctx, ev, ev.Repository.CloneURL, ev.Repository.SSHURL, "github", verifyGithub(signature), payload)
	default:
		return nil
	}
}

func (s *Service) handleGitHubPR(ctx context.Context, ev githubEvent, payload []byte, signature string) error {
	repoURL := ev.Repository.CloneURL
	if repoURL == "" {
		repoURL = ev.Repository.SSHURL
	}
	if repoURL == "" || ev.Number == 0 {
		return nil
	}
	owner, repo := splitFullName(ev.Repository.FullName)
	if owner == "" {
		owner = ev.PR.Head.Repo.Owner.Login
	}
	if repo == "" {
		repo = ev.PR.Head.Repo.Name
	}

	source, err := s.resolveSource(ctx, repoURL, ev.PR.Head.Ref, verifyGithub(signature), payload)
	if err != nil || source == nil {
		return err
	}

	switch ev.Action {
	case "opened", "reopened", "synchronize", "ready_for_review":
		return s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
			PRNumber:  ev.Number,
			PRTitle:   ev.PR.Title,
			PRURL:     ev.PR.HTMLURL,
			Branch:    ev.PR.Head.Ref,
			RepoOwner: owner,
			RepoName:  repo,
			CommitSHA: ev.PR.Head.SHA,
			Source:    "github",
			CreatedBy: &source.UserID,
		})
	case "closed":
		return s.cleanupForPR(ctx, owner, repo, ev.Number)
	default:
		return nil
	}
}

// pushEventPayload is the push shape shared by GitHub and Gitea webhooks.
type pushEventPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
	Commits []struct {
		ID string `json:"id"`
	} `json:"commits"`
}

// handleBranchPush is the shared push pipeline: non-default branch pushes
// create/refresh one preview row per commit; branch deletion destroys them.
// The default branch is compared against payload hints when present.
func (s *Service) handleBranchPush(ctx context.Context, ev pushEventPayload, cloneURL, sshURL, sourceName string, verify sigVerifier, payload []byte) error {
	repoURL := cloneURL
	if repoURL == "" {
		repoURL = sshURL
	}
	branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
	if branch == "" || branch == "refs/heads" {
		return nil
	}
	if strings.EqualFold(branch, findDefaultBranch(payload)) {
		return nil
	}

	source, err := s.resolveSource(ctx, repoURL, branch, verify, payload)
	if err != nil || source == nil {
		return err
	}
	owner, repo := splitFullName(ev.Repository.FullName)

	if ev.Deleted || ev.After == "" || isZeroSHA(ev.After) {
		return s.cleanupForBranch(ctx, owner, repo, branch)
	}

	commits := ev.Commits
	if len(commits) == 0 {
		commits = []struct {
			ID string `json:"id"`
		}{{ID: ev.After}}
	}
	for _, c := range commits {
		if c.ID == "" {
			continue
		}
		err := s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
			PRNumber:  prNumberForBranch(branch, c.ID),
			PRTitle:   previewTitle(branch),
			Branch:    branch,
			RepoOwner: owner,
			RepoName:  repo,
			CommitSHA: c.ID,
			Source:    sourceName,
			CreatedBy: &source.UserID,
		})
		if err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: push preview skipped", "branch", branch, "sha", c.ID, "err", err)
		}
	}
	return nil
}

// ---- GitLab ----

// HandleGitLabWebhookEvent processes object_kind push and merge_request
// payloads; tokenHeader is the X-Gitlab-Token header value.
func (s *Service) HandleGitLabWebhookEvent(ctx context.Context, payload []byte, tokenHeader string) error {
	var ev gitlabEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("gitlab payload: %w", err)
	}

	repoURL := ev.Project.GitHTTPURL
	if repoURL == "" {
		repoURL = ev.Project.GitSSHURL
	}
	namespace, repo := splitFullName(ev.Project.PathWithNS)
	verify := verifyGitLab(tokenHeader)

	switch ev.ObjectKind {
	case "merge_request":
		if repoURL == "" || ev.ObjectAttributes.IID == 0 {
			return nil
		}
		source, err := s.resolveSource(ctx, repoURL, ev.ObjectAttributes.SourceBranch, verify, payload)
		if err != nil || source == nil {
			return err
		}
		switch ev.ObjectAttributes.Action {
		case "open", "reopen", "update":
			sha := ev.ObjectAttributes.SHA
			if sha == "" {
				sha = ev.ObjectAttributes.LastCommit.ID
			}
			return s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
				PRNumber:  ev.ObjectAttributes.IID,
				PRTitle:   ev.ObjectAttributes.Title,
				PRURL:     ev.ObjectAttributes.URL,
				Branch:    ev.ObjectAttributes.SourceBranch,
				RepoOwner: namespace,
				RepoName:  repo,
				CommitSHA: sha,
				Source:    "gitlab",
				CreatedBy: &source.UserID,
			})
		case "close", "merge":
			return s.cleanupForPR(ctx, namespace, repo, ev.ObjectAttributes.IID)
		default:
			return nil
		}
	case "push":
		return s.handleGitLabPush(ctx, ev, repoURL, namespace, repo, verify, payload)
	default:
		return nil
	}
}

func (s *Service) handleGitLabPush(ctx context.Context, ev gitlabEvent, repoURL, namespace, repo string, verify sigVerifier, payload []byte) error {
	branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
	if branch == "" {
		return nil
	}
	if strings.EqualFold(branch, ev.Project.DefaultBranch) {
		return nil
	}
	source, err := s.resolveSource(ctx, repoURL, branch, verify, payload)
	if err != nil || source == nil {
		return err
	}
	if ev.After == "" || isZeroSHA(ev.After) {
		return s.cleanupForBranch(ctx, namespace, repo, branch)
	}
	commits := ev.Commits
	if len(commits) == 0 {
		commits = []struct {
			ID string `json:"id"`
		}{{ID: ev.After}}
	}
	for _, c := range commits {
		if c.ID == "" {
			continue
		}
		err := s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
			PRNumber:  prNumberForBranch(branch, c.ID),
			PRTitle:   previewTitle(branch),
			Branch:    branch,
			RepoOwner: namespace,
			RepoName:  repo,
			CommitSHA: c.ID,
			Source:    "gitlab",
			CreatedBy: &source.UserID,
		})
		if err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: gitlab push preview skipped", "branch", branch, "err", err)
		}
	}
	return nil
}

type gitlabEvent struct {
	ObjectKind string `json:"object_kind"`
	ObjectAttributes struct {
		Action       string `json:"action"`
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		URL          string `json:"url"`
		SourceBranch string `json:"source_branch"`
		SHA          string `json:"sha"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	Project struct {
		GitHTTPURL    string `json:"git_http_url"`
		GitSSHURL     string `json:"git_ssh_url"`
		PathWithNS    string `json:"path_with_namespace"`
		DefaultBranch string `json:"default_branch"`
	} `json:"project"`
	Ref     string `json:"ref"`
	After   string `json:"after"`
	Commits []struct {
		ID string `json:"id"`
	} `json:"commits"`
}

// ---- Bitbucket ----

// HandleBitbucketWebhookEvent handles pullrequest:* and repo:push* events;
// signature is the X-Hub-Signature header value.
func (s *Service) HandleBitbucketEvent(ctx context.Context, payload []byte, eventKey, signature string) error {
	switch {
	case strings.HasPrefix(eventKey, "pullrequest:"):
		var ev bitbucketPReqEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("bitbucket pullrequest payload: %w", err)
		}
		return s.handleBitbucketPR(ctx, ev, payload, signature)
	case strings.HasPrefix(eventKey, "repo:"):
		var ev bitbucketPushEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("bitbucket push payload: %w", err)
		}
		return s.handleBitbucketPush(ctx, ev, payload, signature)
	default:
		return nil
	}
}

type bitbucketPReqEvent struct {
	PullRequest struct {
		ID    int    `json:"id"`
		State string `json:"state"`
		Title string `json:"title"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
		Source struct {
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
			Commit struct {
				Hash string `json:"hash"`
			} `json:"commit"`
		} `json:"source"`
	} `json:"pullrequest"`
	Repository struct {
		FullName string     `json:"full_name"`
		Links    repositoryLinks `json:"links"`
	} `json:"repository"`
}

func (s *Service) handleBitbucketPR(ctx context.Context, ev bitbucketPReqEvent, payload []byte, signature string) error {
	repoURL := cloneURLFrom(ev.Repository.Links.Clone)
	if repoURL == "" || ev.PullRequest.ID == 0 {
		return nil
	}
	owner, repo := splitFullName(ev.Repository.FullName)
	source, err := s.resolveSource(ctx, repoURL, ev.PullRequest.Source.Branch.Name, verifyBitbucket(signature), payload)
	if err != nil || source == nil {
		return err
	}
	switch ev.PullRequest.State {
	case "OPEN":
		return s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
			PRNumber:  ev.PullRequest.ID,
			PRTitle:   ev.PullRequest.Title,
			PRURL:     ev.PullRequest.Links.HTML.Href,
			Branch:    ev.PullRequest.Source.Branch.Name,
			RepoOwner: owner,
			RepoName:  repo,
			CommitSHA: ev.PullRequest.Source.Commit.Hash,
			Source:    "bitbucket",
			CreatedBy: &source.UserID,
		})
	case "MERGED", "DECLINED", "SUPERSEDED":
		return s.cleanupForPR(ctx, owner, repo, ev.PullRequest.ID)
	default:
		return nil
	}
}

type bitbucketPushEvent struct {
	Push struct {
		Changes []struct {
			New struct {
				Name   string `json:"name"`
				Target struct {
					Hash string `json:"hash"`
				} `json:"target"`
			} `json:"new"`
			Old struct {
				Name string `json:"name"`
			} `json:"old"`
			Commits []struct {
				Hash string `json:"hash"`
			} `json:"commits"`
		} `json:"changes"`
	} `json:"push"`
	Repository struct {
		FullName string         `json:"full_name"`
		Links    repositoryLinks `json:"links"`
	} `json:"repository"`
}

func (s *Service) handleBitbucketPush(ctx context.Context, ev bitbucketPushEvent, payload []byte, signature string) error {
	repoURL := cloneURLFrom(ev.Repository.Links.Clone)
	if repoURL == "" || len(ev.Push.Changes) == 0 {
		return nil
	}
	owner, repo := splitFullName(ev.Repository.FullName)
	for _, change := range ev.Push.Changes {
		branch := strings.TrimPrefix(change.New.Name, "refs/heads/")
		if branch == "" {
			branch = strings.TrimPrefix(change.Old.Name, "refs/heads/")
		}
		if branch == "" {
			continue
		}
		source, err := s.resolveSource(ctx, repoURL, branch, verifyBitbucket(signature), payload)
		if err != nil || source == nil {
			return err
		}
		if change.New.Name == "" || change.New.Target.Hash == "" {
			if err := s.cleanupForBranch(ctx, owner, repo, branch); err != nil && s.opts.Logger != nil {
				s.opts.Logger.Warn("previewenv: bitbucket branch cleanup", "branch", branch, "err", err)
			}
			continue
		}
		commit := change.New.Target.Hash
		if len(change.Commits) > 0 {
			commit = change.Commits[len(change.Commits)-1].Hash
		}
		if err := s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
			PRNumber:  prNumberForBranch(branch, commit),
			PRTitle:   previewTitle(branch),
			Branch:    branch,
			RepoOwner: owner,
			RepoName:  repo,
			CommitSHA: commit,
			Source:    "bitbucket",
			CreatedBy: &source.UserID,
		}); err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: bitbucket push preview skipped", "branch", branch, "err", err)
		}
	}
	return nil
}

// ---- Gitea ----

// HandleGiteaEvent processes "push" and "pull_request" events; signature is
// the X-Gitea-Signature header value.
func (s *Service) HandleGiteaEvent(ctx context.Context, payload []byte, event, signature string) error {
	switch event {
	case "pull_request":
		var ev struct {
			Action string `json:"action"`
			Number int    `json:"number"`
			PR     struct {
				Title   string `json:"title"`
				HTMLURL string `json:"html_url"`
				Merged  bool   `json:"merged"`
				Head    struct {
					Ref string `json:"ref"`
					SHA string `json:"sha"`
				} `json:"head"`
			} `json:"pull_request"`
			Repository struct {
				FullName string `json:"full_name"`
				CloneURL string `json:"clone_url"`
				Owner    struct {
					Login string `json:"login"`
				} `json:"owner"`
				Name string `json:"name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("gitea pull_request payload: %w", err)
		}
		if ev.Repository.CloneURL == "" || ev.Number == 0 {
			return nil
		}
		owner, repo := splitFullName(ev.Repository.FullName)
		source, err := s.resolveSource(ctx, ev.Repository.CloneURL, ev.PR.Head.Ref, verifyGitea(signature), payload)
		if err != nil || source == nil {
			return err
		}
		switch ev.Action {
		case "opened", "reopened", "synchronized":
			return s.upsertAndDeploy(ctx, source, &store.PreviewDeployment{
				PRNumber:  ev.Number,
				PRTitle:   ev.PR.Title,
				PRURL:     ev.PR.HTMLURL,
				Branch:    ev.PR.Head.Ref,
				RepoOwner: owner,
				RepoName:  repo,
				CommitSHA: ev.PR.Head.SHA,
				Source:    "gitea",
				CreatedBy: &source.UserID,
			})
		case "closed":
			return s.cleanupForPR(ctx, owner, repo, ev.Number)
		default:
			return nil
		}
	case "push":
		var ev pushEventPayload
		if err := json.Unmarshal(payload, &ev); err != nil {
			return fmt.Errorf("gitea push payload: %w", err)
		}
		return s.handleBranchPush(ctx, ev, ev.Repository.CloneURL, "", "gitea", verifyGitea(signature), payload)
	default:
		return nil
	}
}