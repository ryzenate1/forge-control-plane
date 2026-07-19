package http

import (
	"encoding/json"

	"gamepanel/forge/internal/services/clustermanager"

	"github.com/gofiber/fiber/v2"
)

// safeAuditMeta serializes audit metadata to JSON safely, preventing
// injection via user-controlled values like file paths or backup names.
func safeAuditMeta(kv map[string]string) string {
	b, err := json.Marshal(kv)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func validFileMode(mode string) bool {
	if len(mode) != 3 && len(mode) != 4 {
		return false
	}
	for _, character := range mode {
		if character < '0' || character > '7' {
			return false
		}
	}
	return true
}

func legacyServerTransferUnavailable(c *fiber.Ctx) error {
	return fiber.NewError(fiber.StatusNotImplemented, "legacy server transfer endpoints are not implemented")
}

func legacyServerTransferCallbackUnavailable(c *fiber.Ctx) error {
	return fiber.NewError(fiber.StatusGone, "legacy server transfer callbacks have been retired")
}

func createResourceValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func ensureTransferIdle(c *fiber.Ctx, cfg Config, serverID string) error {
	if cfg.Store == nil {
		return nil
	}
	ctx, cancel := requestContext()
	defer cancel()
	blocked, err := cfg.Store.IsServerTransferBlocking(ctx, serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "server not found")
	}
	if blocked {
		return fiber.NewError(fiber.StatusConflict, "server transfer in progress")
	}
	return nil
}

func registerServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	registerAccessServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerAllocationsServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerBackupsServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerDatabasesServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerFilesServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerIdentityServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerLifecycleServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerMountsServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerSchedulesServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerStartupServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerTransfersServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)

}
