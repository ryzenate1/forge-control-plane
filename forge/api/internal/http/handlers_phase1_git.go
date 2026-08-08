package http

import (
	"context"
	"fmt"
	"strings"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/phase1git"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// Phase 1 "Domain + Git substrate": Git OAuth connect, provider repo
// browsing, the source↔environment map, webhook event map, and deploy-key
// auto-provisioning.

// gitWebhookReceivedType is the event type published for every verified
// provider webhook push/PR change.
const gitWebhookReceivedType = "git.webhook_received"

// ---------- Git OAuth connect ----------

// Phase1GitOAuthAuthorize starts the provider connect flow: it mints a
// single-use state bound to the caller and redirects to the provider's
// authorize page.
func Phase1GitOAuthAuthorize(bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		pt, err := parsePhase1Provider(c.Params("provider"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if !bridge.ProviderConfigured(pt) {
			return fiber.NewError(fiber.StatusBadRequest, "oauth is not configured for this provider")
		}
		authURL, err := bridge.BuildAuthorizeURL(c.UserContext(), pt, claims.Sub, c.Query("redirect_to"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Redirect(authURL, fiber.StatusFound)
	}
}

// Phase1GitOAuthCallback consumes the provider code+state, persists the
// token, and returns the browser to the app.
func Phase1GitOAuthCallback(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		pt, err := parseProvider1(c.Params("provider"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		code, state := c.Query("code"), c.Query("state")
		if code == "" || state == "" {
			return fiber.NewError(fiber.StatusBadRequest, "code and state are required")
		}
		token, target, err := bridge.CompleteOAuth(c.UserContext(), pt, claims.Sub, code, state)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "oauth callback failed: "+err.Error())
		}
		publishGitPhaseEvent(cfg, "git.provider_connected", "git_provider_token", token.ID, fiber.Map{
			"provider": token.Provider,
			"username": token.Username,
		})
		return c.Redirect(safeOAuthTarget(target), fiber.StatusFound)
	}
}

// Phase1GitProviderStatus reports whether OAuth credentials are registered
// for a provider so the UI can show a live "connect" button.
func Phase1GitProviderStatus(bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pt, err := parseProvider1(c.Params("provider"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"provider": pt, "configured": bridge.ProviderConfigured(pt)})
	}
}

// ---------- Provider content browsing ----------

func phase1ProviderToken(c *fiber.Ctx, cfg Config) (store.GitProviderToken, gitprovider.ProviderType, error) {
	if cfg.Store == nil {
		return store.GitProviderToken{}, "", fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	token, err := cfg.Store.GetGitProviderTokenUnmasked(c.UserContext(), c.Params("provider_id"))
	if err != nil {
		return store.GitProviderToken{}, "", fiber.NewError(fiber.StatusNotFound, "provider token not found")
	}
	if err := requireResourceOwner(c, token.UserID); err != nil {
		return store.GitProviderToken{}, "", err
	}
	pt, err := parseProvider1(string(token.Provider))
	if err != nil {
		return store.GitProviderToken{}, "", fiber.NewError(fiber.StatusBadRequest, "unsupported provider")
	}
	return token, pt, nil
}

func repoFullName(c *fiber.Ctx) string {
	return c.Params("owner") + "/" + c.Params("repo")
}

// Phase1GitRepoTree lists a provider repository directory.
func Phase1GitRepoTree(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, pt, err := phase1ProviderToken(c, cfg)
		if err != nil {
			return err
		}
		items, err := bridge.BrowseDir(c.UserContext(), pt, token.AccessToken, token.BaseURL,
			repoFullName(c), c.Query("path"), c.Query("branch"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if items == nil {
			items = []phase1git.RepoItem{}
		}
		return c.JSON(items)
	}
}

// Phase1GitRepoContents returns one file's payload (base64) at a provider.
func Phase1GitRepoContents(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, pt, err := phase1ProviderToken(c, cfg)
		if err != nil {
			return err
		}
		path := c.Query("path")
		if path == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path query parameter is required")
		}
		file, err := bridge.GetFile(c.UserContext(), pt, token.AccessToken, token.BaseURL, repoFullName(c), path, c.Query("branch"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(file)
	}
}

// Phase1GitRepoReadme returns the decoded README for a provider repo.
func Phase1GitRepoReadme(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, pt, err := phase1ProviderToken(c, cfg)
		if err != nil {
			return err
		}
		file, err := bridge.GetReadme(c.UserContext(), pt, token.AccessToken, token.BaseURL, repoFullName(c), c.Query("branch"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(file)
	}
}

// Phase1GitRepoCommits returns the recent commits of a provider repo.
func Phase1GitRepoCommits(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, pt, err := phase1ProviderToken(c, cfg)
		if err != nil {
			return err
		}
		commits, err := bridge.ListCommits(c.UserContext(), pt, token.AccessToken, token.BaseURL,
			repoFullName(c), c.Query("branch"), c.Query("path"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if commits == nil {
			commits = []phase1git.CommitInfo{}
		}
		return c.JSON(commits)
	}
}

// ---------- source ↔ env map ----------

type phase1LinkEnvRequest struct {
	EnvironmentID string `json:"environmentId"`
}

// Phase1GitSourceLinkEnv binds a git source to an environment (the env map).
func Phase1GitSourceLinkEnv(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		var req phase1LinkEnvRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.EnvironmentID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "environmentId is required")
		}
		source, err := cfg.Store.GetGitSource(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		if err := requireResourceOwner(c, source.UserID); err != nil {
			return err
		}
		if linkedOrg := sourceOrgMatch(cfg, ctx, source.ID, req.EnvironmentID); linkedOrg != "" {
			return fiber.NewError(fiber.StatusConflict, "environment belongs to a different organization")
		}
		projectID, orgID, err := cfg.Store.EnvironmentProjectOrg(ctx, req.EnvironmentID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "environment not found")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if ok {
			if canAccess, _ := cfg.Store.CanAccessOrganization(ctx, claims.Subject, orgID); !canAccess {
				return fiber.NewError(fiber.StatusNotFound, "organization not found")
			}
		}
		if err := cfg.Store.LinkGitSourceToEnvironment(ctx, source.ID, req.EnvironmentID); err != nil {
			return respondInternalError(c, err)
		}
		publishGitPhaseEvent(cfg, "git.source_linked", "git_source", source.ID, fiber.Map{
			"sourceId": source.ID, "environmentId": req.EnvironmentID, "projectId": projectID, "orgId": orgID,
		})
		return c.JSON(fiber.Map{"sourceId": source.ID, "environmentId": req.EnvironmentID})
	}
}

// Phase1GitSourceUnlinkEnv removes the source↔environment binding.
func Phase1GitSourceUnlinkEnv(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		source, err := cfg.Store.GetGitSource(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		if err := requireResourceOwner(c, source.UserID); err != nil {
			return err
		}
		if err := cfg.Store.LinkGitSourceToEnvironment(c.UserContext(), source.ID, ""); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// ---------- webhook event map ----------

// Phase1GitSourceWebhookEvents returns the recorded provider events for a
// source (the webhook→event→env map, newest first).
func Phase1GitSourceWebhookEvents(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		source, err := cfg.Store.GetGitSource(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		if err := requireResourceOwner(c, source.UserID); err != nil {
			return err
		}
		events, err := cfg.Store.ListGitWebhookEvents(c.UserContext(), source.ID, 100)
		if err != nil {
			return respondInternalError(c, err)
		}
		if events == nil {
			events = []store.GitWebhookEvent{}
		}
		return c.JSON(events)
	}
}

// ---------- deploy key provisioning ----------

type phase1ProvisionDeployKeyRequest struct {
	Repo  string `json:"repository"`
	Title string `json:"title,omitempty"`
}

// Phase1GitProvisionDeployKey generates a deploy key for a source, stores
// the private half on git_credentials, pushes the public half to the provider
// repository, and binds the credential to the source.
func Phase1GitProvisionDeployKey(cfg Config, bridge *phase1git.Bridge) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.GitService == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git services not available")
		}
		ctx := c.UserContext()
		source, err := cfg.Store.GetGitSource(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		if err := requireResourceOwner(c, source.UserID); err != nil {
			return err
		}
		pt, err := parseProvider1(source.Provider)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "a provider is required for deploy key provisioning")
		}
		if source.ProviderTokenID == nil || *source.ProviderTokenID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "git source has no linked provider token")
		}
		token, err := cfg.Store.GetGitProviderTokenUnmasked(ctx, *source.ProviderTokenID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "provider token unavailable")
		}

		var req phase1ProvisionDeployKeyRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		repo := req.Repo
		if repo == "" {
			repo = source.RepositoryOwner + "/" + source.RepositoryName
		}

		kp, err := cfg.GitService.GenerateDeployKeyPair("ed25519", 0)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		cred, err := cfg.Store.CreateGitCredential(ctx, store.CreateGitCredentialRequest{
			UserID:         source.UserID,
			Name:           fmt.Sprintf("deploy-%s-%s", source.RepositoryName, shortID(source.ID)),
			CredentialType: store.GitCredentialSSHKey,
			Credential:     kp.PrivateKey,
			PublicKey:      kp.PublicKey,
			Description:    "phase1 auto-provisioned deploy key",
		})
		if err != nil {
			return respondInternalError(c, err)
		}

		if pushed, err := bridge.AutoprovisionDeployKey(ctx, pt, token.AccessToken, token.BaseURL, repo, req.Title, kp.PublicKey); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		} else {
			if err := cfg.Store.UpdateGitSourceCredentialID(ctx, source.ID, cred.ID); err != nil {
				return respondInternalError(c, err)
			}
			publishGitPhaseEvent(cfg, "git.deploy_key_provisioned", "git_source", source.ID, fiber.Map{
				"sourceId": source.ID, "credentialId": cred.ID, "repository": repo, "keyId": pushed.ID,
			})
			return c.JSON(fiber.Map{"credentialId": cred.ID, "publicKey": kp.PublicKey, "providerKeyId": pushed.ID})
		}
	}
}

// ---------- helpers ----------

func parseProvider1(pt string) (gitprovider.ProviderType, error) {
	switch gitprovider.ProviderType(pt) {
	case gitprovider.ProviderGitHub, gitprovider.ProviderGitLab, gitprovider.ProviderBitbucket, gitprovider.ProviderGitea:
		return gitprovider.ProviderType(pt), nil
	}
	return "", fmt.Errorf("unsupported provider: %s", pt)
}

// sourceOrgMatch returns the source's org id when it disagrees with the env's
// owning org; empty string means "compatible".
func sourceOrgMatch(cfg Config, ctx, sourceID, envID string) string {
	link, err := cfg.Store.GetGitSourceLink(ctx, sourceID)
	if err != nil || link.OrgID == nil || *link.OrgID == "" {
		return ""
	}
	_, orgID, err := cfg.Store.EnvironmentProjectOrg(ctx, envID)
	if err != nil || orgID == "" {
		return ""
	}
	if *link.OrgID != orgID {
		return *link.OrgID
	}
	return ""
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// safeOAuthTarget only allows relative paths so the OAuth callback can never
// bounce the browser to an attacker-controlled origin.
func safeOAuthTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "//") {
		return "/"
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return "/"
	}
	return target
}

// publishGitPhaseEvent publishes a phase event onto the shared event
// registry in the background so slow subscribers never block request paths.
func publishGitPhaseEvent(cfg Config, eventType, resourceType, resourceID string, payload fiber.Map) {
	if cfg.EventRegistry == nil {
		return
	}
	if payload == nil {
		payload = fiber.Map{}
	}
	env := events.NewEnvelope(events.EventType(eventType), "git", resourceType, resourceID, payload)
	_ = cfg.EventRegistry.Publish(context.Background(), env)
}

// recordGitWebhookEvent persists a provider webhook hit on the event map and
// publishes the structured git.webhook_received event.
func recordGitWebhookEvent(cfg Config, event store.GitWebhookEvent) {
	if cfg.Store != nil {
		if err := cfg.Store.CreateGitWebhookEvent(context.Background(), event); err != nil && cfg.Logger != nil {
			cfg.Logger.Warn("git webhook event record failed", "error", err.Error())
		}
	}
	payload := map[string]any{
		"provider": event.Provider,
		"event":    event.EventType,
		"repo":     event.Repository,
		"ref":      event.Ref,
		"sha":      event.SHA,
		"prNumber": event.PRNumber,
		"action":   event.Action,
		"installationId": event.InstallationID,
	}
	resourceID := event.Repository
	if event.SourceID != nil {
		resourceID = *event.SourceID
	}
	publishGitPhaseEvent(cfg, gitWebhookReceivedType, "git_source", resourceID, payload)
}

var _ = events.WildcardEventType