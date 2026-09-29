package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gamepanel/forge/internal/store"
)

// CommitStatus is a provider-agnostic commit status/check report posted back
// to a git provider so PRs surface the preview environment lifecycle.
// State is one of "pending", "success", "failure" (GitHub/GitLab/Gitea) and
// is translated per provider (Bitbucket build states) inside ReportCommitStatus.
type CommitStatus struct {
	State       string `json:"state"` // pending | success | failure
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

// ReportCommitStatus posts a commit status through the provider REST client
// used by the rest of the git service. token comes from an unmasked provider
// token row; baseURL is optional (empty == public SaaS endpoint). This is the
// only entry point statuses ship through, so all providers share the same
// verification/URL validation rules as repo discovery.
func (s *Service) ReportCommitStatus(ctx context.Context, provider store.GitProviderType, token, baseURL, owner, repo, sha string, st CommitStatus) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("report commit status: access token is required")
	}
	if err := validateProviderBaseURL(provider, baseURL); err != nil {
		return err
	}

	switch provider {
	case store.GitProviderGitHub:
		return s.postGitHubStatus(ctx, token, baseURL, owner, repo, sha, st)
	case store.GitProviderGitLab:
		return s.postGitLabStatus(ctx, token, baseURL, owner, repo, sha, st)
	case store.GitProviderBitbucket:
		return s.postBitbucketStatus(ctx, token, baseURL, owner, repo, sha, st)
	case store.GitProviderGitea:
		return s.postGiteaStatus(ctx, token, baseURL, owner, repo, sha, st)
	default:
		return fmt.Errorf("report commit status: unsupported provider: %s", provider)
	}
}

// ReportCommitStatusForUser posts a status through the caller's explicit
// provider token. It resolves the token by ID so multi-token accounts cannot
// silently report under the wrong credential.
func (s *Service) ReportCommitStatusForUser(ctx context.Context, userID string, provider store.GitProviderType, owner, repo, sha string, st CommitStatus) error {
	return s.ReportCommitStatusForToken(ctx, userID, "", provider, owner, repo, sha, st)
}

// ReportCommitStatusForToken posts a status through an explicit provider token
// ID. When tokenID is empty, exactly one usable token must exist for
// (user, provider); zero yields not-found and more than one yields an
// ambiguity error instead of silently picking the first.
func (s *Service) ReportCommitStatusForToken(ctx context.Context, userID, tokenID string, provider store.GitProviderType, owner, repo, sha string, st CommitStatus) error {
	if userID == "" || owner == "" || repo == "" || sha == "" {
		return fmt.Errorf("report commit status for user: user, repo and sha are required")
	}
	if s.store == nil {
		return fmt.Errorf("report commit status: store is not available")
	}
	if tokenID != "" {
		unmasked, err := s.store.GetGitProviderTokenUnmasked(ctx, tokenID)
		if err != nil {
			return fmt.Errorf("report commit status: load token: %w", err)
		}
		if unmasked.UserID != userID {
			return fmt.Errorf("report commit status: provider token not found")
		}
		if unmasked.Provider != provider {
			return fmt.Errorf("report commit status: token is for %s, not %s", unmasked.Provider, provider)
		}
		if unmasked.AccessToken == "" {
			return fmt.Errorf("report commit status: token has no access token")
		}
		return s.ReportCommitStatus(ctx, unmasked.Provider, unmasked.AccessToken, unmasked.BaseURL, owner, repo, sha, st)
	}
	tokens, err := s.store.ListGitProviderTokens(ctx, userID)
	if err != nil {
		return fmt.Errorf("report commit status: list tokens: %w", err)
	}
	var candidates []store.GitProviderToken
	for _, t := range tokens {
		if t.Provider == provider {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return fmt.Errorf("report commit status: no usable %s provider token for user %s", provider, userID)
	}
	if len(candidates) > 1 {
		return fmt.Errorf("report commit status: %d %s provider tokens for user %s — specify providerTokenId", len(candidates), provider, userID)
	}
	unmasked, err := s.store.GetGitProviderTokenUnmasked(ctx, candidates[0].ID)
	if err != nil {
		return fmt.Errorf("report commit status: load token: %w", err)
	}
	if unmasked.AccessToken == "" {
		return fmt.Errorf("report commit status: token has no access token")
	}
	return s.ReportCommitStatus(ctx, unmasked.Provider, unmasked.AccessToken, unmasked.BaseURL, owner, repo, sha, st)
}

// -------- per-provider payloads --------

func (s *Service) postGitHubStatus(ctx context.Context, token, baseURL, owner, repo, sha string, st CommitStatus) error {
	apiBase := githubAPIBase(baseURL)
	path := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", apiBase, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))
	body, _ := json.Marshal(map[string]string{
		"state":        st.State,
		"target_url":   st.TargetURL,
		"description":  st.Description,
		"context":      st.Context,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Forge-Git/1.0")
	return s.doHTTPRequest(req, "github status")
}

func (s *Service) postGitLabStatus(ctx context.Context, token, baseURL, owner, repo, sha string, st CommitStatus) error {
	api := gitLabAPIBase(baseURL)
	project := url.PathEscape(owner + "/" + repo)
	path := fmt.Sprintf("%s/projects/%s/statuses/%s", api, project, url.PathEscape(sha))

	form := url.Values{}
	if st.State != "" {
		form.Set("state", st.State)
	}
	if st.TargetURL != "" {
		form.Set("target_url", st.TargetURL)
	}
	if st.Description != "" {
		form.Set("description", st.Description)
	}
	if st.Context != "" {
		form.Set("name", st.Context)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Git-Git/1.0")
	return s.doHTTPRequest(req, "gitlab status")
}

func (s *Service) postBitbucketStatus(ctx context.Context, token, baseURL, owner, repo, sha string, st CommitStatus) error {
	api := bitbucketAPIBase(baseURL)
	path := fmt.Sprintf("%s/repositories/%s/%s/commit/%s/statuses/build", api, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))

	key := "forge-preview"
	if st.Context != "" {
		key = st.Context
	}
	state := "INPROGRESS"
	switch st.State {
	case "success":
		state = "SUCCESSFUL"
	case "failure":
		state = "FAILED"
	}
	body, _ := json.Marshal(map[string]string{
		"key":         key,
		"state":       state,
		"url":         st.TargetURL,
		"description": st.Description,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth("x-token-auth", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Git-Git/1.0")
	return s.doHTTPRequest(req, "bitbucket status")
}

func (s *Service) postGiteaStatus(ctx context.Context, token, baseURL, owner, repo, sha string, st CommitStatus) error {
	if baseURL == "" {
		return fmt.Errorf("gitea status: base URL is required")
	}
	api := strings.TrimRight(baseURL, "/") + "/api/v1"
	path := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", api, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))
	body, _ := json.Marshal(map[string]string{
		"state":       st.State,
		"target_url":  st.TargetURL,
		"description": st.Description,
		"context":     st.Context,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Git-Git/1.0")
	return s.doHTTPRequest(req, "gitea status")
}

func (s *Service) doHTTPRequest(req *http.Request, label string) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d", label, resp.StatusCode)
	}
	return nil
}