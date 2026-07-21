package gitprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

type ProviderType string

const (
	ProviderGitHub    ProviderType = "github"
	ProviderGitLab    ProviderType = "gitlab"
	ProviderBitbucket ProviderType = "bitbucket"
	ProviderGitea     ProviderType = "gitea"
	ProviderGeneric   ProviderType = "generic"
)

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string
}

type ProviderConfig struct {
	Type          ProviderType
	Name          string
	OAuth         *OAuthConfig
	BaseURL       string // API base URL (for Gitea / self-hosted)
	WebhookSecret string
}

type Service struct {
	store  *store.Store
	logger *slog.Logger
	client *http.Client
	config map[ProviderType]*ProviderConfig
}

func NewService(s *store.Store, logger *slog.Logger) *Service {
	return &Service{
		store:  s,
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
		config: make(map[ProviderType]*ProviderConfig),
	}
}

func (s *Service) RegisterProviderConfig(cfg *ProviderConfig) {
	s.config[cfg.Type] = cfg
}

func (s *Service) GetProviderConfig(pt ProviderType) *ProviderConfig {
	return s.config[pt]
}

func (s *Service) OAuthAuthorizeURL(pt ProviderType, state string) (string, error) {
	cfg, ok := s.config[pt]
	if !ok || cfg.OAuth == nil {
		return "", fmt.Errorf("oauth not configured for provider %s", pt)
	}

	var authURL string
	switch pt {
	case ProviderGitHub:
		authURL = "https://github.com/login/oauth/authorize"
	case ProviderGitLab:
		authURL = "https://gitlab.com/oauth/authorize"
	case ProviderBitbucket:
		authURL = "https://bitbucket.org/site/oauth2/authorize"
	case ProviderGitea:
		base := strings.TrimRight(cfg.BaseURL, "/")
		authURL = base + "/login/oauth/authorize"
	default:
		return "", fmt.Errorf("unsupported provider: %s", pt)
	}

	q := url.Values{}
	q.Set("client_id", cfg.OAuth.ClientID)
	q.Set("redirect_uri", cfg.OAuth.RedirectURL)
	q.Set("response_type", "code")
	q.Set("state", state)

	scopes := cfg.OAuth.Scopes
	if scopes == "" {
		scopes = defaultScopes(pt)
	}
	q.Set("scope", scopes)

	return authURL + "?" + q.Encode(), nil
}

func defaultScopes(pt ProviderType) string {
	switch pt {
	case ProviderGitHub:
		return "repo,user,admin:repo_hook"
	case ProviderGitLab:
		return "api,read_repository,write_repository"
	case ProviderBitbucket:
		return "repository:write,webhook,pullrequest:write"
	case ProviderGitea:
		return "read,write,offline_access"
	default:
		return ""
	}
}

func (s *Service) ExchangeOAuthCode(ctx context.Context, pt ProviderType, code string) (*OAuthTokenResult, error) {
	cfg, ok := s.config[pt]
	if !ok || cfg.OAuth == nil {
		return nil, fmt.Errorf("oauth not configured for provider %s", pt)
	}

	tokenURL := oauthTokenURL(pt, cfg)
	data := url.Values{}
	data.Set("client_id", cfg.OAuth.ClientID)
	data.Set("client_secret", cfg.OAuth.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", cfg.OAuth.RedirectURL)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var result OAuthTokenResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.AccessToken == "" {
		return nil, errors.New("no access token in response")
	}

	if result.TokenType == "" {
		result.TokenType = "bearer"
	}

	return &result, nil
}

func oauthTokenURL(pt ProviderType, cfg *ProviderConfig) string {
	switch pt {
	case ProviderGitHub:
		return "https://github.com/login/oauth/access_token"
	case ProviderGitLab:
		return "https://gitlab.com/oauth/token"
	case ProviderBitbucket:
		return "https://bitbucket.org/site/oauth2/access_token"
	case ProviderGitea:
		base := strings.TrimRight(cfg.BaseURL, "/")
		return base + "/login/oauth/access_token"
	default:
		return ""
	}
}

type OAuthTokenResult struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

type UserInfo struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
}

func (s *Service) GetUserInfo(ctx context.Context, pt ProviderType, accessToken string, baseURL string) (*UserInfo, error) {
	var apiURL string
	switch pt {
	case ProviderGitHub:
		apiURL = "https://api.github.com/user"
	case ProviderGitLab:
		apiURL = "https://gitlab.com/api/v4/user"
	case ProviderBitbucket:
		apiURL = "https://api.bitbucket.org/2.0/user"
	case ProviderGitea:
		apiURL = strings.TrimRight(baseURL, "/") + "/api/v1/user"
	default:
		apiURL = strings.TrimRight(baseURL, "/") + "/api/v1/user"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("user info request failed: %s", string(body))
	}

	info := &UserInfo{}
	switch pt {
	case ProviderGitHub, ProviderGitea:
		var gh struct {
			ID        int    `json:"id"`
			Login     string `json:"login"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.Unmarshal(body, &gh); err != nil {
			return nil, err
		}
		info.ID = fmt.Sprintf("%d", gh.ID)
		info.Username = gh.Login
		info.Email = gh.Email
		info.AvatarURL = gh.AvatarURL
	case ProviderGitLab:
		var gl struct {
			ID        int    `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := json.Unmarshal(body, &gl); err != nil {
			return nil, err
		}
		info.ID = fmt.Sprintf("%d", gl.ID)
		info.Username = gl.Username
		info.Email = gl.Email
		info.AvatarURL = gl.AvatarURL
	case ProviderBitbucket:
		var bb struct {
			UUID     string `json:"uuid"`
			Nickname string `json:"nickname"`
			Links    struct {
				Avatar struct {
					Href string `json:"href"`
				} `json:"avatar"`
			} `json:"links"`
		}
		if err := json.Unmarshal(body, &bb); err != nil {
			return nil, err
		}
		info.ID = bb.UUID
		info.Username = bb.Nickname
		info.AvatarURL = bb.Links.Avatar.Href
	}

	return info, nil
}

func (s *Service) ListRepositories(ctx context.Context, pt ProviderType, accessToken, baseURL string) ([]store.GitProviderRepo, error) {
	var apiURL string
	switch pt {
	case ProviderGitHub:
		apiURL = "https://api.github.com/user/repos?per_page=100&sort=updated"
	case ProviderGitLab:
		apiURL = "https://gitlab.com/api/v4/projects?per_page=100&membership=true"
	case ProviderBitbucket:
		apiURL = "https://api.bitbucket.org/2.0/repositories?role=member&pagelen=100"
	case ProviderGitea:
		apiURL = strings.TrimRight(baseURL, "/") + "/api/v1/user/repos?limit=100"
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list repos failed: %s", string(body))
	}

	var repos []store.GitProviderRepo
	switch pt {
	case ProviderGitHub:
		var ghRepos []struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			CloneURL      string `json:"clone_url"`
			SSHURL        string `json:"ssh_url"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
			Description   string `json:"description"`
		}
		if err := json.Unmarshal(body, &ghRepos); err != nil {
			return nil, err
		}
		for _, r := range ghRepos {
			repos = append(repos, store.GitProviderRepo{
				Name:          r.Name,
				FullName:      r.FullName,
				CloneURL:      r.CloneURL,
				SSHURL:        r.SSHURL,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				Description:   r.Description,
			})
		}
	case ProviderGitLab:
		var glRepos []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			PathWithNS  string `json:"path_with_namespace"`
			HTTPURL     string `json:"http_url_to_repo"`
			SSHURL      string `json:"ssh_url_to_repo"`
			DefaultBr   string `json:"default_branch"`
			Visibility  string `json:"visibility"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(body, &glRepos); err != nil {
			return nil, err
		}
		for _, r := range glRepos {
			repos = append(repos, store.GitProviderRepo{
				Name:          r.Name,
				FullName:      r.PathWithNS,
				CloneURL:      r.HTTPURL,
				SSHURL:        r.SSHURL,
				DefaultBranch: r.DefaultBr,
				Private:       r.Visibility == "private",
				Description:   r.Description,
			})
		}
	case ProviderBitbucket:
		var bbResp struct {
			Values []struct {
				Name        string `json:"name"`
				FullName    string `json:"full_name"`
				Links       struct {
					Clone []struct {
						Name string `json:"name"`
						Href string `json:"href"`
					} `json:"clone"`
				} `json:"links"`
				MainBranch struct {
					Name string `json:"name"`
				} `json:"mainbranch"`
				IsPrivate   bool   `json:"is_private"`
				Description string `json:"description"`
			} `json:"values"`
		}
		if err := json.Unmarshal(body, &bbResp); err != nil {
			return nil, err
		}
		for _, r := range bbResp.Values {
			cloneURL := ""
			sshURL := ""
			for _, c := range r.Links.Clone {
				if c.Name == "https" {
					cloneURL = c.Href
				}
				if c.Name == "ssh" {
					sshURL = c.Href
				}
			}
			repos = append(repos, store.GitProviderRepo{
				Name:          r.Name,
				FullName:      r.FullName,
				CloneURL:      cloneURL,
				SSHURL:        sshURL,
				DefaultBranch: r.MainBranch.Name,
				Private:       r.IsPrivate,
				Description:   r.Description,
			})
		}
	case ProviderGitea:
		var gRepos []struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			CloneURL      string `json:"clone_url"`
			SSHURL        string `json:"ssh_url"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
			Description   string `json:"description"`
		}
		if err := json.Unmarshal(body, &gRepos); err != nil {
			return nil, err
		}
		for _, r := range gRepos {
			repos = append(repos, store.GitProviderRepo{
				Name:          r.Name,
				FullName:      r.FullName,
				CloneURL:      r.CloneURL,
				SSHURL:        r.SSHURL,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				Description:   r.Description,
			})
		}
	}

	return repos, nil
}

func (s *Service) ListBranches(ctx context.Context, pt ProviderType, accessToken, baseURL, repoOwner, repoName string) ([]store.GitProviderBranch, error) {
	var apiURL string
	repoPath := repoOwner + "/" + repoName
	switch pt {
	case ProviderGitHub:
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/branches?per_page=100", repoPath)
	case ProviderGitLab:
		apiURL = fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/repository/branches", url.PathEscape(repoPath))
	case ProviderBitbucket:
		apiURL = fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/refs/branches?pagelen=100", repoPath)
	case ProviderGitea:
		apiURL = strings.TrimRight(baseURL, "/") + fmt.Sprintf("/api/v1/repos/%s/branches?limit=100", repoPath)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", pt)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list branches failed: %s", string(body))
	}

	var branches []store.GitProviderBranch
	switch pt {
	case ProviderGitHub, ProviderGitea:
		var ghBranches []struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(body, &ghBranches); err != nil {
			return nil, err
		}
		for _, b := range ghBranches {
			branches = append(branches, store.GitProviderBranch{Name: b.Name, SHA: b.Commit.SHA, IsMain: b.Name == "main" || b.Name == "master"})
		}
	case ProviderGitLab:
		var glBranches []struct {
			Name string `json:"name"`
			Commit struct {
				ID string `json:"id"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(body, &glBranches); err != nil {
			return nil, err
		}
		for _, b := range glBranches {
			branches = append(branches, store.GitProviderBranch{Name: b.Name, SHA: b.Commit.ID, IsMain: b.Name == "main" || b.Name == "master"})
		}
	case ProviderBitbucket:
		var bbResp struct {
			Values []struct {
				Name string `json:"name"`
				Target struct {
					Hash string `json:"hash"`
				} `json:"target"`
			} `json:"values"`
		}
		if err := json.Unmarshal(body, &bbResp); err != nil {
			return nil, err
		}
		for _, b := range bbResp.Values {
			branches = append(branches, store.GitProviderBranch{Name: b.Name, SHA: b.Target.Hash, IsMain: b.Name == "main" || b.Name == "master"})
		}
	}

	return branches, nil
}

func (s *Service) RegisterWebhook(ctx context.Context, pt ProviderType, accessToken, baseURL, repoOwner, repoName, webhookURL, webhookSecret string) (string, error) {
	var apiURL string
	repoPath := repoOwner + "/" + repoName
	switch pt {
	case ProviderGitHub:
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/hooks", repoPath)
	case ProviderGitLab:
		apiURL = fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/hooks", url.PathEscape(repoPath))
	case ProviderBitbucket:
		apiURL = fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/hooks", repoPath)
	case ProviderGitea:
		apiURL = strings.TrimRight(baseURL, "/") + fmt.Sprintf("/api/v1/repos/%s/hooks", repoPath)
	default:
		return "", fmt.Errorf("unsupported provider: %s", pt)
	}

	payload := map[string]any{
		"url":    webhookURL,
		"active": true,
	}

	switch pt {
	case ProviderGitHub:
		payload["name"] = "web"
		payload["events"] = []string{"push"}
		payload["config"] = map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       webhookSecret,
		}
	case ProviderGitLab:
		payload["push_events"] = true
		payload["token"] = webhookSecret
		payload["enable_ssl_verification"] = true
	case ProviderBitbucket:
		payload["description"] = "GamePanel auto-deploy"
		payload["events"] = map[string]any{
			"push": map[string]bool{"enabled": true},
		}
	case ProviderGitea:
		payload["type"] = "gitea"
		payload["events"] = []string{"push"}
		payload["config"] = map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       webhookSecret,
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("webhook registration failed: %s", string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if pt == ProviderGitHub || pt == ProviderGitea {
		json.Unmarshal(respBody, &result)
	} else if pt == ProviderGitLab {
		var gl struct {
			ID int `json:"id"`
		}
		json.Unmarshal(respBody, &gl)
		result.ID = gl.ID
	} else if pt == ProviderBitbucket {
		var bb struct {
			UUID string `json:"uuid"`
		}
		json.Unmarshal(respBody, &bb)
		return bb.UUID, nil
	}

	return fmt.Sprintf("%d", result.ID), nil
}

func (s *Service) DeleteWebhook(ctx context.Context, pt ProviderType, accessToken, baseURL, repoOwner, repoName, webhookID string) error {
	var apiURL string
	repoPath := repoOwner + "/" + repoName
	switch pt {
	case ProviderGitHub:
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/hooks/%s", repoPath, webhookID)
	case ProviderGitLab:
		apiURL = fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/hooks/%s", url.PathEscape(repoPath), webhookID)
	case ProviderBitbucket:
		apiURL = fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/hooks/%s", repoPath, webhookID)
	case ProviderGitea:
		apiURL = strings.TrimRight(baseURL, "/") + fmt.Sprintf("/api/v1/repos/%s/hooks/%s", repoPath, webhookID)
	default:
		return fmt.Errorf("unsupported provider: %s", pt)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *Service) CreateOAuthState(ctx context.Context, userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)
	// Store state in the git_providers table or a separate cache
	// For simplicity we use a convention: store a temporary entry
	return state, nil
}

func (s *Service) ConnectProvider(ctx context.Context, userID string, pt ProviderType, token *OAuthTokenResult, userInfo *UserInfo, baseURL string) (store.GitProviderToken, error) {
	expiresAt := (*time.Time)(nil)
	if token.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	metadata, _ := json.Marshal(map[string]string{
		"connected_at": time.Now().UTC().Format(time.RFC3339),
	})

	return s.store.CreateGitProviderToken(ctx, store.CreateGitProviderTokenRequest{
		UserID:       userID,
		Provider:     store.GitProviderType(pt),
		ProviderName: string(pt) + "-" + userInfo.Username,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        token.Scope,
		BaseURL:      baseURL,
		Username:     userInfo.Username,
		AvatarURL:    userInfo.AvatarURL,
		Metadata:     metadata,
	})
}

// Generic providers
func (s *Service) CreateGenericProvider(ctx context.Context, userID, name, accessToken, baseURL string) (store.GitProviderToken, error) {
	metadata, _ := json.Marshal(map[string]string{
		"connected_at": time.Now().UTC().Format(time.RFC3339),
	})

	return s.store.CreateGitProviderToken(ctx, store.CreateGitProviderTokenRequest{
		UserID:       userID,
		Provider:     store.GitProviderGeneric,
		ProviderName: name,
		AccessToken:  accessToken,
		TokenType:    "bearer",
		Scope:        "api",
		BaseURL:      baseURL,
		Username:     name,
		Metadata:     metadata,
	})
}
