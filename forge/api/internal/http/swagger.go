package http

import (
	"path/filepath"
	"runtime"

	"github.com/gofiber/fiber/v2"
)

func registerSwaggerRoutes(app *fiber.App) {
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))

	app.Get("/api/docs/openapi.json", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(baseDir, "docs", "openapi.json"))
	})

	app.Get("/api/docs/openapi.yaml", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(baseDir, "docs", "openapi.yaml"))
	})

	app.Static("/api/docs", filepath.Join(baseDir, "docs", "swagger-ui"))
}
