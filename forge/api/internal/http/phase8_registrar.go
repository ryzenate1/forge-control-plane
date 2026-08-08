package http

import (
	"gamepanel/forge/internal/services/forgefile"
	"gamepanel/forge/internal/services/onboarding"

	"github.com/gofiber/fiber/v2"
)

// RegisterPhase8 is the Phase 8 (env-as-code + polish) registrar. It wires
// the forge.yaml apply/validate surface, the onboarding wizard endpoints,
// branding upload + persistence, and the IDE file listing onto the shared
// routers. It only depends on services already present in http.Config
// (Store, GitService, AppHostingService) and constructs its own lightweight
// facades, so an incomplete shared config degrades to 5xx instead of
// crashing route registration.
func RegisterPhase8(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	st := cfg.Store

	ffSvc := forgefile.NewService(st, cfg.AppHostingService, cfg.Logger, forgefileBaseDomain())
	onbSvc := onboarding.NewService(st, cfg.GitService, cfg.AppHostingService, cfg.Logger)

	admin := protected.Group("/admin", requireRole("admin"))
	admin.Post("/branding/upload", brandingUploadHandler(cfg, false))
	admin.Post("/settings/branding-upload", brandingUploadHandler(cfg, true))
	admin.Get("/branding/file/:name", brandingFileHandler(cfg))

	protected.Get("/forgefile", listForgefileManifestsHandler(ffSvc))
	protected.Get("/forgefile/validate", validateForgefileHandler(ffSvc, false))
	protected.Post("/forgefile/validate", validateForgefileHandler(ffSvc, false))
	protected.Post("/forgefile/apply", applyForgefileHandler(ffSvc))
	protected.Get("/forgefile/:slug", getForgefileHandler(ffSvc))
	v1.Get("/public/forgefile/:slug", publicForgefileHandler(ffSvc))

	onb := protected.Group("/onboarding")
	onb.Get("/status", onboardingStatusHandler(onbSvc))
	onb.Get("/repos", onboardingReposHandler(onbSvc))
	onb.Get("/repos/:repo/branches", onboardingBranchesHandler(onbSvc))
	onb.Post("/connect", onboardingConnectHandler(onbSvc))
	onb.Post("/deploy", onboardingDeployHandler(onbSvc))

	protected.Get("/ide/files", ideFilesHandler(onbSvc))

	return nil
}