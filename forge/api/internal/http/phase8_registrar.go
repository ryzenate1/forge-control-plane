package http

import (
	"github.com/gofiber/fiber/v2"
)

// Phase registrar priorities are spaced so a phase can be slotted between two
// others without renumbering. Existing phases are phase1-git at 40,
// phase3-catalog at 100 and phase2-environment-engine at 2000; those numbers
// are arbitrary but their relative order is what Fiber uses to resolve
// overlapping paths, so they are left alone.
const phase8Priority = 800

func init() {
	RegisterPhaseRegistrar("phase8-env-as-code", phase8Priority, RegisterPhase8)
}

// RegisterPhase8 registers the branding upload/serve surface, which has no
// other owner.
//
// It used to also register /forgefile/*, /onboarding/* and /ide/files. Those
// paths belong to registerForgefileRoutes (handlers_forgefile.go) and
// registerPhase8OnboardingRoutes (phase8_onboarding.go), both of which run
// before this registrar — every register*Routes call happens first, and among
// registrars the onboarding one has the lower priority number — so Fiber
// resolved all of those paths to the other registration and these copies never
// answered a request. They also carried weaker gating: the forgefile copies
// had no requireRole at all, and the manifest read was additionally exposed
// unauthenticated on v1 as /public/forgefile/:slug, leaking a project's deploy
// configuration and environment list to anyone who could guess a slug. The
// duplicates are gone and the two behaviours worth keeping (raw-YAML bodies,
// GET-form validate) now live on the routes that actually serve.
func RegisterPhase8(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg == nil || cfg.Store == nil {
		return nil
	}

	admin := protected.Group("/admin", requireRole("admin"))
	admin.Post("/branding/upload", brandingUploadHandler(cfg, false))
	admin.Post("/settings/branding-upload", brandingUploadHandler(cfg, true))
	admin.Get("/branding/file/:name", brandingFileHandler(cfg))

	return nil
}
