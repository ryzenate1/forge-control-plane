package http

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// ---------- Git Credentials ----------

type CreateGitCredentialRequest struct {
	Name           string `json:"name"`
	CredentialType string `json:"credentialType"`
	Credential     string `json:"credential"`
	Description    string `json:"description"`
}

func ListGitCredentials(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		creds, err := cfg.Store.ListGitCredentials(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(creds)
	}
}

func CreateGitCredential(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		var req CreateGitCredentialRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		cred, err := cfg.Store.CreateGitCredential(ctx, store.CreateGitCredentialRequest{
			UserID:         claims.Sub,
			Name:           req.Name,
			CredentialType: store.GitCredentialType(req.CredentialType),
			Credential:     req.Credential,
			Description:    req.Description,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(cred)
	}
}

func GetGitCredential(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		cred, err := cfg.Store.GetGitCredential(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(cred)
	}
}

func DeleteGitCredential(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteGitCredential(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// ---------- Deploy Key Generation ----------

func GenerateDeployKey(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.GitService == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		kp, err := cfg.GitService.GenerateDeployKey(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(kp)
	}
}

// ---------- Git Provider Tokens ----------

type CreateProviderTokenRequest struct {
	Provider     string `json:"provider"`
	ProviderName string `json:"providerName"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	BaseURL      string `json:"baseUrl"`
	Username     string `json:"username"`
}

func ListGitProviderTokens(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tokens, err := cfg.Store.ListGitProviderTokens(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(tokens)
	}
}

func ConnectGitProvider(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		var req CreateProviderTokenRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		token, err := cfg.Store.CreateGitProviderToken(ctx, store.CreateGitProviderTokenRequest{
			UserID:       claims.Sub,
			Provider:     store.GitProviderType(req.Provider),
			ProviderName: req.ProviderName,
			AccessToken:  req.AccessToken,
			RefreshToken: req.RefreshToken,
			TokenType:    req.TokenType,
			BaseURL:      req.BaseURL,
			Username:     req.Username,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(token)
	}
}

func DisconnectGitProvider(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteGitProviderToken(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func ListProviderRepos(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.GitService == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		repos, err := cfg.GitService.ListProviderRepos(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(repos)
	}
}

func ListProviderBranches(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.GitService == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		repoFullName := c.Query("repo")
		if repoFullName == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repo query parameter is required")
		}
		branches, err := cfg.GitService.ListProviderBranches(ctx, c.Params("id"), repoFullName)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(branches)
	}
}

// ---------- Git Sources ----------

type CreateGitSourceRequest struct {
	CredentialID    *string `json:"credentialId"`
	ProviderTokenID *string `json:"providerTokenId"`
	Provider        string  `json:"provider"`
	RepositoryURL   string  `json:"repositoryUrl"`
	RepositoryName  string  `json:"repositoryName"`
	RepositoryOwner string  `json:"repositoryOwner"`
	Branch          string  `json:"branch"`
	AutoDeploy      bool    `json:"autoDeploy"`
}

func ListGitSources(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		sources, err := cfg.Store.ListGitSources(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(sources)
	}
}

func CreateGitSource(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		var req CreateGitSourceRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		webhookSecret := generateWebhookSecret()
		webhookID := ""

		if req.AutoDeploy && req.ProviderTokenID != nil && *req.ProviderTokenID != "" && cfg.GitService != nil {
			webhookURL := buildWebhookURL(c, req.Provider)
			ctx, cancel := requestContext()
			defer cancel()
			id, err := cfg.GitService.SetupProviderWebhook(ctx, *req.ProviderTokenID, req.RepositoryOwner+"/"+req.RepositoryName, webhookURL, webhookSecret)
			if err != nil {
				if cfg.Logger != nil {
					cfg.Logger.Warn("failed to setup provider webhook", "err", err)
				}
			} else {
				webhookID = id
			}
		}

		ctx, cancel := requestContext()
		defer cancel()
		source, err := cfg.Store.CreateGitSource(ctx, store.CreateGitSourceRequest{
			UserID:          claims.Sub,
			CredentialID:    req.CredentialID,
			ProviderTokenID: req.ProviderTokenID,
			Provider:        req.Provider,
			RepositoryURL:   req.RepositoryURL,
			RepositoryName:  req.RepositoryName,
			RepositoryOwner: req.RepositoryOwner,
			Branch:          req.Branch,
			AutoDeploy:      req.AutoDeploy,
			WebhookSecret:   webhookSecret,
			WebhookID:       webhookID,
			WebhookURL:      buildWebhookURL(c, req.Provider),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(source)
	}
}

func DeleteGitSource(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteGitSource(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// ---------- Webhook Handlers ----------

type webhookPushPayload struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
}

func HandleGitHubWebhook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		body := c.Body()
		signature := c.Get("X-Hub-Signature-256")

		event := c.Get("X-GitHub-Event")
		if event != "push" {
			return c.SendStatus(fiber.StatusOK)
		}

		var payload webhookPushPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}

		ctx, cancel := requestContext()
		defer cancel()

		repoURL := payload.Repository.CloneURL
		if repoURL == "" {
			repoURL = payload.Repository.SSHURL
		}
		branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

		source, err := cfg.Store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
		if err != nil || source == nil {
			return c.SendStatus(fiber.StatusOK)
		}

		if err := git.VerifyGitHubSignature(body, signature, source.WebhookSecret); err != nil && source.WebhookSecret != "" {
			if cfg.Logger != nil {
				cfg.Logger.Warn("github webhook signature verification failed", "repo", repoURL, "branch", branch)
			}
			return c.SendStatus(fiber.StatusOK)
		}

		if len(payload.Commits) > 0 && cfg.GitService != nil {
			lastCommit := payload.Commits[len(payload.Commits)-1]
			_ = cfg.GitService.HandleWebhookTrigger(ctx, source, lastCommit.ID, lastCommit.Message, lastCommit.Author.Name)
		} else if cfg.GitService != nil {
			_ = cfg.GitService.HandleWebhookTrigger(ctx, source, payload.After, "", "")
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

func HandleGitLabWebhook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		body := c.Body()
		tokenHeader := c.Get("X-Gitlab-Token")

		var payload struct {
			ObjectKind string `json:"object_kind"`
			Ref        string `json:"ref"`
			CheckoutSHA string `json:"checkout_sha"`
			Project    struct {
				GitHTTPURL string `json:"git_http_url"`
				GitSSHURL  string `json:"git_ssh_url"`
			} `json:"project"`
			Commits []struct {
				ID      string `json:"id"`
				Message string `json:"message"`
				Author  struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"author"`
			} `json:"commits"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}

		if payload.ObjectKind != "push" && payload.ObjectKind != "" {
			return c.SendStatus(fiber.StatusOK)
		}

		ctx, cancel := requestContext()
		defer cancel()

		repoURL := payload.Project.GitHTTPURL
		if repoURL == "" {
			repoURL = payload.Project.GitSSHURL
		}
		branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

		source, err := cfg.Store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
		if err != nil || source == nil {
			return c.SendStatus(fiber.StatusOK)
		}

		if err := git.VerifyGitLabSignature(body, tokenHeader, source.WebhookSecret); err != nil && source.WebhookSecret != "" {
			if cfg.Logger != nil {
				cfg.Logger.Warn("gitlab webhook signature verification failed", "repo", repoURL)
			}
			return c.SendStatus(fiber.StatusOK)
		}

		sha := payload.CheckoutSHA
		msg := ""
		author := ""
		if len(payload.Commits) > 0 {
			sha = payload.Commits[len(payload.Commits)-1].ID
			msg = payload.Commits[len(payload.Commits)-1].Message
			author = payload.Commits[len(payload.Commits)-1].Author.Name
		}

		if cfg.GitService != nil {
			_ = cfg.GitService.HandleWebhookTrigger(ctx, source, sha, msg, author)
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

func HandleBitbucketWebhook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		body := c.Body()

		event := c.Get("X-Event-Key")
		if event != "repo:push" {
			return c.SendStatus(fiber.StatusOK)
		}

		var payload struct {
			Push struct {
				Changes []struct {
					New struct {
						Name   string `json:"name"`
						Target struct {
							Hash string `json:"hash"`
						} `json:"target"`
					} `json:"new"`
					Commits []struct {
						Hash    string `json:"hash"`
						Message string `json:"message"`
						Author  struct {
							User struct {
								DisplayName string `json:"display_name"`
							} `json:"user"`
						} `json:"author"`
					} `json:"commits"`
				} `json:"changes"`
			} `json:"push"`
			Repository struct {
				FullName string `json:"full_name"`
				Links    struct {
					Clone []struct {
						Name string `json:"name"`
						Href string `json:"href"`
					} `json:"clone"`
				} `json:"links"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}

		ctx, cancel := requestContext()
		defer cancel()

		repoURL := ""
		for _, link := range payload.Repository.Links.Clone {
			if link.Name == "https" {
				repoURL = link.Href
				break
			}
		}
		if repoURL == "" {
			return c.SendStatus(fiber.StatusOK)
		}

		if len(payload.Push.Changes) == 0 {
			return c.SendStatus(fiber.StatusOK)
		}

		change := payload.Push.Changes[0]
		branch := strings.TrimPrefix(change.New.Name, "refs/heads/")

		source, err := cfg.Store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
		if err != nil || source == nil {
			return c.SendStatus(fiber.StatusOK)
		}

		signature := c.Get("X-Hub-Signature")
		if err := git.VerifyBitbucketSignature(body, signature, source.WebhookSecret); err != nil && source.WebhookSecret != "" {
			if cfg.Logger != nil {
				cfg.Logger.Warn("bitbucket webhook signature verification failed", "repo", repoURL)
			}
			return c.SendStatus(fiber.StatusOK)
		}

		sha := change.New.Target.Hash
		msg := ""
		author := ""
		if len(change.Commits) > 0 {
			sha = change.Commits[len(change.Commits)-1].Hash
			msg = change.Commits[len(change.Commits)-1].Message
			author = change.Commits[len(change.Commits)-1].Author.User.DisplayName
		}

		if cfg.GitService != nil {
			_ = cfg.GitService.HandleWebhookTrigger(ctx, source, sha, msg, author)
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

func HandleGiteaWebhook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		body := c.Body()
		signature := c.Get("X-Gitea-Signature")

		var payload struct {
			Ref        string `json:"ref"`
			After      string `json:"after"`
			Repository struct {
				FullName string `json:"full_name"`
				CloneURL string `json:"clone_url"`
				SSHURL   string `json:"ssh_url"`
			} `json:"repository"`
			Commits []struct {
				ID      string `json:"id"`
				Message string `json:"message"`
				Author  struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"author"`
			} `json:"commits"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
		}

		ctx, cancel := requestContext()
		defer cancel()

		repoURL := payload.Repository.CloneURL
		if repoURL == "" {
			repoURL = payload.Repository.SSHURL
		}
		branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

		source, err := cfg.Store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
		if err != nil || source == nil {
			return c.SendStatus(fiber.StatusOK)
		}

		if err := git.VerifyGiteaSignature(body, signature, source.WebhookSecret); err != nil && source.WebhookSecret != "" {
			if cfg.Logger != nil {
				cfg.Logger.Warn("gitea webhook signature verification failed", "repo", repoURL)
			}
			return c.SendStatus(fiber.StatusOK)
		}

		sha := payload.After
		msg := ""
		author := ""
		if len(payload.Commits) > 0 {
			sha = payload.Commits[len(payload.Commits)-1].ID
			msg = payload.Commits[len(payload.Commits)-1].Message
			author = payload.Commits[len(payload.Commits)-1].Author.Name
		}

		if cfg.GitService != nil {
			_ = cfg.GitService.HandleWebhookTrigger(ctx, source, sha, msg, author)
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

func buildWebhookURL(c *fiber.Ctx, provider string) string {
	base := string(c.Request().URI().Scheme()) + "://" + string(c.Request().URI().Host())
	return fmt.Sprintf("%s/api/v1/git/webhook/%s", base, provider)
}

func generateWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ---------- Git Deployments ----------

type CreateDeploymentRequest struct {
	RepoURL    string `json:"repoUrl"`
	Branch     string `json:"branch"`
	CommitHash string `json:"commitHash"`
}

func ListGitDeployments(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		source, err := cfg.Store.GetGitSourceByServerID(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		deploys, err := cfg.Store.ListGitDeployments(ctx, source.ID, 10)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if deploys == nil {
			deploys = []store.GitDeployment{}
		}
		return c.JSON(deploys)
	}
}

func GetGitDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		deploy, err := cfg.Store.GetGitDeployment(ctx, c.Params("deployment_id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(deploy)
	}
}

func TriggerGitDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateDeploymentRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.RepoURL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoUrl is required")
		}
		if req.Branch == "" {
			req.Branch = "main"
		}
		ctx, cancel := requestContext()
		defer cancel()
		source, err := cfg.Store.FindGitSourceByRepoAndBranch(ctx, req.RepoURL, req.Branch)
		if err != nil || source == nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found for the given repository and branch")
		}
		deploy, err := cfg.Store.CreateGitDeployment(ctx, store.CreateGitDeploymentRequest{
			GitSourceID: source.ID,
			Branch:     req.Branch,
			CommitSHA:  req.CommitHash,
			Status:     "pending",
			StartedAt:  time.Now().UTC(),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(deploy)
	}
}

// ---------- Git Deployment Hooks ----------

type CreateGitDeploymentHookRequest struct {
	Events []string `json:"events"`
}

func ListGitDeploymentHooks(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		source, err := cfg.Store.GetGitSourceByServerID(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		hooks, err := cfg.Store.ListGitDeploymentHooks(ctx, source.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if hooks == nil {
			hooks = []store.GitDeploymentHook{}
		}
		return c.JSON(hooks)
	}
}

func CreateGitDeploymentHook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateGitDeploymentHookRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Events == nil {
			req.Events = []string{"push"}
		}
		secret := ""
		if cfg.GitDeployMgmtService != nil {
			secret = cfg.GitDeployMgmtService.GenerateSecret()
		}
		ctx, cancel := requestContext()
		defer cancel()
		source, err := cfg.Store.GetGitSourceByServerID(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}
		hook, err := cfg.Store.CreateGitDeploymentHook(ctx, source.ID, secret, req.Events)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(hook)
	}
}

func DeleteGitDeploymentHook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteGitDeploymentHook(ctx, c.Params("hook_id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func RegenerateGitDeploymentHookSecret(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		secret := ""
		if cfg.GitDeployMgmtService != nil {
			secret = cfg.GitDeployMgmtService.GenerateSecret()
		}
		ctx, cancel := requestContext()
		defer cancel()
		hook, err := cfg.Store.RegenerateGitDeploymentHookSecret(ctx, c.Params("hook_id"), secret)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(hook)
	}
}

// ---------- Webhook Receiver for Git Deployments ----------

type receiveWebhookRequest struct {
	ServerID string `json:"serverId"`
}

func ReceiveGitDeploymentWebhook(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.SendStatus(fiber.StatusOK)
		}
		serverID := c.Params("serverId")
		if serverID == "" {
			var req receiveWebhookRequest
			if err := c.BodyParser(&req); err == nil && req.ServerID != "" {
				serverID = req.ServerID
			}
		}
		if serverID == "" {
			return c.SendStatus(fiber.StatusOK)
		}

		signature := c.Get("X-Hub-Signature-256")
		if signature == "" {
			signature = c.Get("X-Hub-Signature")
		}

		body := c.Body()

		ctx, cancel := requestContext()
		defer cancel()

		if cfg.GitDeployMgmtService != nil {
			if err := cfg.GitDeployMgmtService.HandleWebhookPayload(ctx, serverID, "", body, signature); err != nil {
				if cfg.Logger != nil {
					cfg.Logger.Warn("git deployment webhook processing failed", "server", serverID, "err", err)
				}
			}
		}

		return c.SendStatus(fiber.StatusOK)
	}
}

func registerGitRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	gitGroup := protected.Group("/git")

	gitGroup.Get("/credentials", ListGitCredentials(cfg))
	gitGroup.Post("/credentials", mutationLimiter, CreateGitCredential(cfg))
	gitGroup.Get("/credentials/:id", GetGitCredential(cfg))
	gitGroup.Delete("/credentials/:id", mutationLimiter, DeleteGitCredential(cfg))
	gitGroup.Post("/credentials/:id/generate-key", mutationLimiter, GenerateDeployKey(cfg))

	gitGroup.Get("/providers", ListGitProviderTokens(cfg))
	gitGroup.Post("/providers", mutationLimiter, ConnectGitProvider(cfg))
	gitGroup.Delete("/providers/:id", mutationLimiter, DisconnectGitProvider(cfg))
	gitGroup.Get("/providers/:id/repos", ListProviderRepos(cfg))
	gitGroup.Get("/providers/:id/branches", ListProviderBranches(cfg))

	gitGroup.Get("/sources", ListGitSources(cfg))
	gitGroup.Post("/sources", mutationLimiter, CreateGitSource(cfg))
	gitGroup.Delete("/sources/:id", mutationLimiter, DeleteGitSource(cfg))

	// ---- Git Deployment routes ----
	gitGroup.Get("/servers/:id/deployments", ListGitDeployments(cfg))
	gitGroup.Post("/servers/:id/deployments", mutationLimiter, TriggerGitDeployment(cfg))
	gitGroup.Get("/servers/:id/deployments/:deployment_id", GetGitDeployment(cfg))

	gitGroup.Get("/servers/:id/hooks", ListGitDeploymentHooks(cfg))
	gitGroup.Post("/servers/:id/hooks", mutationLimiter, CreateGitDeploymentHook(cfg))
	gitGroup.Post("/servers/:id/hooks/:hook_id/regenerate", mutationLimiter, RegenerateGitDeploymentHookSecret(cfg))
	gitGroup.Delete("/servers/:id/hooks/:hook_id", mutationLimiter, DeleteGitDeploymentHook(cfg))
}

func registerGitWebhookRoutes(v1 fiber.Router, cfg Config) {
	v1.Post("/git/webhook/github", HandleGitHubWebhook(cfg))
	v1.Post("/git/webhook/gitlab", HandleGitLabWebhook(cfg))
	v1.Post("/git/webhook/bitbucket", HandleBitbucketWebhook(cfg))
	v1.Post("/git/webhook/gitea", HandleGiteaWebhook(cfg))
	v1.Post("/git/webhook/deploy/:serverId", ReceiveGitDeploymentWebhook(cfg))
}
