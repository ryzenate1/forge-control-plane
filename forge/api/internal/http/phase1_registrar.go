package http

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/phase1git"

	"github.com/gofiber/fiber/v2"
)

func init() {
	RegisterPhaseRegistrar("phase1-git", 40, func(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
		if cfg == nil {
			return fmt.Errorf("phase1-git: nil config")
		}
		registerPhase1ProviderConfigs(cfg)
		bridge := phase1git.New(cfg.GitProviderService, cfg.Store, cfg.Logger)

		git := protected.Group("/git")

		git.Get("/oauth/:provider/authorize", Phase1GitOAuthAuthorize(bridge))
		git.Get("/oauth/:provider/callback", Phase1GitOAuthCallback(cfg, bridge))
		git.Get("/oauth/:provider/status", Phase1GitProviderStatus(bridge))

		git.Get("/providers/:providerId/repos/:owner/:repo/tree", Phase1GitRepoTree(cfg, bridge))
		git.Get("/providers/:providerId/repos/:owner/:repo/contents", Phase1GitRepoContents(cfg, bridge))
		git.Get("/providers/:providerId/repos/:owner/:repo/readme", Phase1GitRepoReadme(cfg, bridge))
		git.Get("/providers/:providerId/repos/:owner/:repo/commits", Phase1GitRepoCommits(cfg, bridge))

		git.Post("/sources/:id/link-env", Phase1GitSourceLinkEnv(cfg))
		git.Delete("/sources/:id/link-env", Phase1GitSourceUnlinkEnv(cfg))
		git.Get("/sources/:id/webhook-events", Phase1GitSourceWebhookEvents(cfg))
		git.Post("/sources/:id/provision-deploy-key", Phase1GitProvisionDeployKey(cfg, bridge))

		return nil
	})
}

// registerPhase1OAuthConfigs wires the gitprovider.Service's OAuth configs
// from GIT_* env vars, mirroring the per-service env() convention. Providers
// without a client id/secret pair are skipped (authorize returns 400).
func registerPhase1OAuthConfigs(cfg *Config) {
	if cfg.GitProviderService == nil {
		return
	}
	providers := []struct {
		pt       gitprovider.ProviderType
		name     string
		idEnv    string
		secretEnv string
		baseEnv  string
	}{
		{gitprovider.ProviderGitHub, "GitHub", "GIT_GITHUB_CLIENT_ID", "GIT_GITHUB_CLIENT_SECRET", ""},
		{gitprovider.ProviderGitLab, "GitLab", "GIT_GITLAB_CLIENT_ID", "GIT_GITLAB_CLIENT_SECRET", ""},
		{gitprovider.ProviderBitbucket, "Bitbucket", "GIT_BITBUCKET_CLIENT_ID", "GIT_BITBUCKET_CLIENT_SECRET", ""},
		{gitprovider.ProviderGitea, "Gitea", "GIT_GITEA_CLIENT_ID", "GIT_GITEA_CLIENT_SECRET", "GIT_GITEA_BASE_URL"},
	}
	for _, p := range providers {
		id := strings.TrimSpace(os.Getenv(p.idEnv))
		secret := strings.TrimSpace(os.Getenv(p.secretEnv))
		if id == "" || secret == "" {
			continue
		}
		baseURL := ""
		if p.baseEnv != "" {
			baseURL = strings.TrimSpace(os.Getenv(p.baseEnv))
		}
		cfg.GitProviderService.RegisterProviderConfig(&gitprovider.ProviderConfig{
			Type:    p.pt,
			Name:    p.name,
			BaseURL: baseURL,
			OAuth: &gitprovider.OAuthConfig{
				ClientID:     id,
				ClientSecret: secret,
				RedirectURL:  phase1OAuthRedirectURL(cfg, string(p.pt)),
			},
		})
	}
}

// phase1OAuthRedirectURL is the callback URL the provider bounces the user's
// browser back to. Registered once per provider at startup so authorize and
// callback always agree.
func phase1OAuthRedirectURL(cfg *Config, provider string) string {
	base, err := url.Parse(strings.TrimSpace(cfg.PanelURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/git/oauth/" + url.PathEscape(provider) + "/callback"
	base.RawQuery = ""
	base.Fragment = ""
	return base.String()
}