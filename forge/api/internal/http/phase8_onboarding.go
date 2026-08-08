package http

import (
	"strings"

	"gamepanel/forge/internal/services/onboarding"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func currentClaims(c *fiber.Ctx) (tokenClaims, error) {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}
	return claims, nil
}

// GET /api/v1/onboarding/status — wizard state for the current user.
func onboardingStatusHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		return c.JSON(svc.Status(ctx, claims.Sub))
	}
}

// GET /api/v1/onboarding/repos — repository autocomplete listing.
func onboardingReposHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		repos, err := svc.ListRepos(ctx, claims.Sub, c.Query("providerId"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if repos == nil {
			repos = []store.GitProviderRepo{}
		}
		return c.JSON(fiber.Map{"data": repos})
	}
}

// GET /api/v1/onboarding/repos/:repo/branches — branch listing for a repo.
func onboardingBranchesHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		repo := strings.TrimSuffix(c.Params("repo"), ".git")
		if repo == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repo is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		branches, err := svc.ListBranches(ctx, claims.Sub, c.Query("providerId"), repo)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if branches == nil {
			branches = []store.GitProviderBranch{}
		}
		return c.JSON(fiber.Map{"data": branches})
	}
}

// POST /api/v1/onboarding/connect — paste-a-token connect form.
func onboardingConnectHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		var req onboarding.ConnectRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		token, err := svc.Connect(ctx, claims.Sub, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"providerToken": token})
	}
}

// POST /api/v1/onboarding/deploy — template pick → app + demo URL.
func onboardingDeployHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		var req onboarding.DeployRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.RepoFullName) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repo is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.Deploy(ctx, claims.Sub, claims.Role, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(result)
	}
}

// GET /api/v1/ide/files — IDE surface mock tree: loaded git sources.
func ideFilesHandler(svc *onboarding.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := currentClaims(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		sources, err := svc.ListLoadedSources(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if sources == nil {
			sources = []onboarding.LoadedSource{}
		}
		return c.JSON(fiber.Map{"data": sources})
	}
}