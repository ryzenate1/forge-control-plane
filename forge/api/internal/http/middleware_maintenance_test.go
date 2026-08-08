package http

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestMaintenanceMode_DisabledByDefault(t *testing.T) {
	origMode := os.Getenv("FORGE_MAINTENANCE_MODE")
	origBypass := os.Getenv("FORGE_MAINTENANCE_BYPASS_TOKEN")
	defer os.Setenv("FORGE_MAINTENANCE_MODE", origMode)
	defer os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", origBypass)
	os.Unsetenv("FORGE_MAINTENANCE_MODE")

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MaintenanceModeMiddleware(Config{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 when maintenance mode is disabled, got %d", resp.StatusCode)
	}
}

func TestMaintenanceMode_EnabledReturns503(t *testing.T) {
	origMode := os.Getenv("FORGE_MAINTENANCE_MODE")
	origBypass := os.Getenv("FORGE_MAINTENANCE_BYPASS_TOKEN")
	defer os.Setenv("FORGE_MAINTENANCE_MODE", origMode)
	defer os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", origBypass)
	os.Setenv("FORGE_MAINTENANCE_MODE", "true")

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MaintenanceModeMiddleware(Config{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 when maintenance mode is enabled, got %d", resp.StatusCode)
	}
}

func TestMaintenanceMode_EnabledWithValidBypassHeader(t *testing.T) {
	origMode := os.Getenv("FORGE_MAINTENANCE_MODE")
	origBypass := os.Getenv("FORGE_MAINTENANCE_BYPASS_TOKEN")
	defer os.Setenv("FORGE_MAINTENANCE_MODE", origMode)
	defer os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", origBypass)
	os.Setenv("FORGE_MAINTENANCE_MODE", "true")
	os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", "my-bypass-token")

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MaintenanceModeMiddleware(Config{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forge-Maintenance-Bypass", "my-bypass-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with valid bypass header, got %d", resp.StatusCode)
	}
}

func TestMaintenanceMode_EnabledWithInvalidBypassHeader(t *testing.T) {
	origMode := os.Getenv("FORGE_MAINTENANCE_MODE")
	origBypass := os.Getenv("FORGE_MAINTENANCE_BYPASS_TOKEN")
	defer os.Setenv("FORGE_MAINTENANCE_MODE", origMode)
	defer os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", origBypass)
	os.Setenv("FORGE_MAINTENANCE_MODE", "true")
	os.Setenv("FORGE_MAINTENANCE_BYPASS_TOKEN", "my-bypass-token")

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MaintenanceModeMiddleware(Config{}))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forge-Maintenance-Bypass", "wrong-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 with invalid bypass header, got %d", resp.StatusCode)
	}
}
