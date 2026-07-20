package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestPlatformRoutesExposeDefaultScopeAndRequirePostgresForMutations(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", tokenClaims{Sub: "admin", Role: "admin"}); return c.Next() })
	registerPlatformRoutes(app, Config{}, func(c *fiber.Ctx) error { return c.Next() }, func(c *fiber.Ctx) error { return c.Next() })

	response, err := app.Test(httptest.NewRequest("GET", "/platform/scope/default", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusOK)
	}
	response.Body.Close()

	response, err = app.Test(httptest.NewRequest("GET", "/platform/workloads", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusServiceUnavailable)
	}
	response.Body.Close()

	response, err = app.Test(httptest.NewRequest("GET", "/platform/applications", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusServiceUnavailable)
	}
	response.Body.Close()

	response, err = app.Test(httptest.NewRequest("GET", "/platform/applications/example", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusServiceUnavailable)
	}
	response.Body.Close()
}
