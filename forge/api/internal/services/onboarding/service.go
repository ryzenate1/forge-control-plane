// Package onboarding implements the Phase 8 "connect repo → get URL" wizard
// backend. It wraps the existing git service (provider repo listing) and
// apphosting service (app creation) behind /api/v1/onboarding/* endpoints so
// the demo path completes fast without touching apphosting internals.
// Deploys are scaffolded: no remote registry push happens; a "deploy
// queued" record plus a demo URL is returned.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gamepanel/forge/internal/services/apphosting"
	gitsvc "gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

// Env knobs for the onboarding demo path (workshop extension points).
const (
	// DemoURLEnv overrides the fake demo URL base returned by Deploy.
	DemoURLEnv = "FORGE_DEMO_URL"
	// GithubClientIDEnv enables the rendered GitHub OAuth link.
	GithubClientIDEnv = "GITHUB_CLIENT_ID"
	// GithubAuthorizeURLEnv overrides https://github.com/login/oauth/authorize.
	GithubAuthorizeURLEnv = "GITHUB_OAUTH_AUTHORIZE_URL"
	// GithubRedirectURIEnv is appended to the OAuth link when set.
	GithubRedirectURIEnv = "GITHUB_REDIRECT_URI"
)

// Builder resolution for the wizard's template pick:
// Static/Node/Next/Nixpacks/Dockerfile -> forge buildbuckets.
var templateBuilders = map[string]string{
	"static":     "static",
	"node":       "nixpacks",
	"next":       "nixpacks",
	"nixpacks":   "nixpacks",
	"dockerfile": "dockerfile",
	"heroku":     "heroku",
}

// StatusView is returned by GET /onboarding/status.
type StatusView struct {
	Connected    bool     `json:"connected"`
	Providers    []string `json:"providers"`
	SourceCount  int      `json:"sourceCount"`
	HasApps      bool     `json:"hasApps"`
	TemplateKeys []string `json:"templateKeys"`
	OAuthURL     string   `json:"oauthUrl,omitempty"`
}

// ConnectRequest is the paste-a-token form of the wizard.
type ConnectRequest struct {
	Provider     string `json:"provider"`
	ProviderName string `json:"providerName"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	BaseURL      string `json:"baseUrl"`
	Username     string `json:"username"`
}

// DeployRequest drives POST /onboarding/deploy.
type DeployRequest struct {
	ProviderTokenID string `json:"providerTokenId,omitempty"`
	RepoFullName    string `json:"repo"`
	Branch          string `json:"branch,omitempty"`
	Root            string `json:"root,omitempty"`
	BuildType       string `json:"buildType"`
	Dockerfile      string `json:"dockerfile,omitempty"`
	AppName         string `json:"name,omitempty"`
	Port            int    `json:"port,omitempty"`
}

// DeployResult is the "deploy queued + URL" payload.
type DeployResult struct {
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
	AppID        string `json:"appId"`
	AppName      string `json:"appName"`
	URL          string `json:"url"`
	InternalURL  string `json:"internalUrl"`
	Builder      string `json:"builder"`
}

// LoadedSource is one entry of GET /ide/files (the IDE surface mock list).
type LoadedSource struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Owner         string   `json:"owner"`
	Branch        string   `json:"branch"`
	CloneURL      string   `json:"cloneUrl"`
	DefaultBranch string   `json:"defaultBranch"`
	Files         []string `json:"files"`
}

// Service is the onboarding facade over the shared git + apphosting
// services.
type Service struct {
	store       *store.Store
	gitSvc      *gitsvc.Service
	appSvc      *apphosting.Service
	logger      *slog.Logger
	demoURLBase string
}

func NewService(st *store.Store, gitSvc *gitsvc.Service, appSvc *apphosting.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:       st,
		gitSvc:      gitSvc,
		appSvc:      appSvc,
		logger:      logger,
		demoURLBase: strings.TrimRight(strings.TrimSpace(os.Getenv(DemoURLEnv)), "/"),
	}
}

func (s *Service) demoURLBase() string {
	if s.demoURLBase == "" {
		return "http://localhost:3000/deploy"
	}
	return s.demoURLBase
}

// GithubOAuthLink renders the GitHub authorize URL for the wizard's
// "Connect GitHub" button. Returns "" when GITHUB_CLIENT_ID is unset.
func (s *Service) GitHubOAuthLink() string {
	clientID := strings.TrimSpace(os.Getenv(GithubClientIDEnv))
	if clientID == "" {
		return ""
	}
	authorize := strings.TrimSpace(os.Getenv(GithubAuthorizeURLEnv))
	if authorize == "" {
		authorize = "https://github.com/login/oauth/authorize"
	}
	q := "?client_id=" + urlQueryEscape(clientID) + "&scope=repo%20read:org&response_type=code"
	if redirect := strings.TrimSpace(os.Getenv(GithubRedirectURIEnv)); redirect != "" {
		q += "&redirect_uri=" + urlQueryEscape(redirect)
	}
	return authorize + q
}

func urlQueryEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "&", "%26"), "=", "%3D")
}

// Status summarizes what the current user has already connected.
func (s *Service) Status(ctx context.Context, userID string) StatusView {
	view := StatusView{TemplateKeys: templateKeys()}
	view.OAuthURL = s.GitHubOAuthLink()
	if s.store == nil {
		return view
	}
	if tokens, err := s.store.ListGitProviderTokens(ctx, userID); err == nil {
		for _, t := range tokens {
			view.Providers = append(view.Providers, string(t.Provider))
			view.Connected = true
		}
	}
	if sources, err := s.store.ListGitSources(ctx, userID); err == nil {
		view.SourceCount = len(sources)
	}
	return view
}

func templateKeys() []string {
	return []string{"static", "node", "next", "nixpacks", "dockerfile", "heroku"}
}

// ListRepos lists provider repos for the user (autocomplete listing source).
// providerTokenID may be empty to auto-pick the user's first token.
func (s *Service) ListRepos(ctx context.Context, userID, providerTokenID string) ([]store.GitProviderRepo, error) {
	if s.gitSvc == nil || s.store == nil {
		return nil, errors.New("git service is not available")
	}
	tokenID := strings.TrimSpace(providerTokenID)
	if tokenID == "" {
		tokens, err := s.store.ListGitProviderTokens(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, t := range tokens {
			tokenID = t.ID
			break
		}
		if tokenID == "" {
			return nil, errors.New("no git provider connected — paste a token or configure GITHUB_CLIENT_ID first")
		}
	}
	return s.gitSvc.ListProviderRepos(ctx, tokenID)
}

// ListBranches returns branches for a repo owned by the user's token.
func (s *Service) ListBranches(ctx context.Context, userID, providerTokenID, repoFullName string) ([]store.GitProviderBranch, error) {
	if s.gitSvc == nil || s.store == nil {
		return nil, errors.New("git service is not available")
	}
	if strings.TrimSpace(providerTokenID) == "" {
		return nil, errors.New("providerTokenId is required")
	}
	token, err := s.store.GetGitProviderToken(ctx, providerTokenID)
	if err != nil {
		return nil, err
	}
	if token.UserID != userID {
		return nil, errors.New("provider token not found")
	}
	return s.gitSvc.ListProviderBranches(ctx, providerTokenID, repoFullName)
}

// Connect stores a pasted provider token.
func (s *Service) Connect(ctx context.Context, userID string, req ConnectRequest) (*store.GitProviderToken, error) {
	if s.store == nil {
		return nil, errors.New("postgres is required")
	}
	provider := strings.TrimSpace(req.Provider)
	accessToken := strings.TrimSpace(req.AccessToken)
	if provider == "" {
		return nil, errors.New("provider is required")
	}
	if accessToken == "" {
		return nil, errors.New("accessToken is required")
	}
	token, err := s.store.CreateGitProviderToken(ctx, store.CreateGitProviderTokenRequest{
		UserID:       userID,
		Provider:     store.GitProviderType(provider),
		ProviderName: strings.TrimSpace(req.ProviderName),
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    "bearer",
		Scope:        "repo,read:org",
		BaseURL:      req.BaseURL,
		Username:     req.Username,
	})
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// Deploy scaffolds the app: no remote registry push (instant demo path by
// design); a queued deployment id and the demo URL are returned.
func (s *Service) Deploy(ctx context.Context, userID, role string, req DeployRequest) (*DeployResult, error) {
	if s.appSvc == nil {
		return nil, errors.New("app hosting service is not available")
	}
	builder, ok := templateBuilders[strings.ToLower(strings.TrimSpace(req.BuildType))]
	if !ok {
		builder = "nixpacks"
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	name := strings.TrimSpace(req.AppName)
	if name == "" {
		name = slugFromRepo(req.RepoFullName)
	}
	if name == "" {
		return nil, errors.New("app name or repository is required")
	}

	orgID := "default"
	if role != "admin" && s.store != nil {
		if orgs, err := s.store.ListOrganizationsForUser(ctx, userID); err == nil && len(orgs) > 0 {
			orgID = orgs[0].ID
		}
	}

	sourceCfg, _ := json.Marshal(map[string]any{
		"repository": req.RepoFullName,
		"repo":       req.RepoFullName,
		"branch":     branch,
		"root":       strings.TrimSpace(req.Root),
		"builder":    builder,
		"dockerfile": req.Dockerfile,
		"port":       req.Port,
	})

	app, err := s.appSvc.CreateApp(ctx, tenancy.OrgContext{OrgID: orgID, Role: "admin"}, apphosting.CreateAppRequest{
		Name:         name,
		Description:  "created from onboarding wizard",
		OrgID:        orgID,
		SourceType:   "GIT",
		SourceConfig: sourceCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("create app: %w", err)
	}

	port := req.Port
	if port == 0 {
		port = 8080
	}
	if _, err := s.appSvc.CreateService(ctx, app.ID, orgID, apphosting.CreateServiceRequest{
		Name:     name,
		Replicas: 1,
		Ports:    []store.AppPort{{ContainerPort: port, Protocol: "tcp"}},
		EnvVars:  map[string]string{},
	}); err != nil {
		s.logger.Warn("onboarding: service scaffold failed, app remains", "appID", app.ID, "error", err)
	}

	deploymentID := uuid.NewString()
	return &DeployResult{
		DeploymentID: deploymentID,
		Status:       "queued",
		AppID:        app.ID,
		AppName:      app.Name,
		URL:          s.demoURLBase() + "/" + deploymentID,
		InternalURL:  "http://localhost:3000/deploy/" + deploymentID,
		Builder:      builder,
	}, nil
}

// ListLoadedSources is the IDE surface mock listing: the git sources the
// user has loaded plus conventional editable entrypoints.
func (s *Service) ListLoadedSources(ctx context.Context, userID string) ([]LoadedSource, error) {
	if s.store == nil {
		return nil, errors.New("store is unavailable")
	}
	sources, err := s.store.ListGitSources(ctx, userID)
	if err != nil {
		return nil, err
	}
	entries := make([]LoadedSource, 0, len(sources))
	for _, src := range sources {
		entries = append(entries, LoadedSource{
			ID:            src.ID,
			Name:          src.RepositoryName,
			Owner:         src.RepositoryOwner,
			Branch:        src.Branch,
			CloneURL:      src.RepositoryURL,
			DefaultBranch: src.Branch,
			Files:         []string{"Dockerfile", "package.json", "src/", "README.md"},
		})
	}
	return entries, nil
}

func slugFromRepo(full string) string {
	parts := strings.Split(strings.TrimSuffix(full, ".git"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		return ""
	}
	re := strings.NewReplacer("_", "-", ".", "-", " ", "-")
	return strings.ToLower(re.Replace(name))
}

// keepImportAnchor silences unused-import lints when uuid is vendored lean.
