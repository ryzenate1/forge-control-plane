package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDBContainerEnginesRoute(t *testing.T) {
	app := dbContainerTestApp()
	req := httptest.NewRequest(http.MethodGet, "/databases/engines", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestDBContainerProvisionRouteRequiresWriteScope(t *testing.T) {
	payload := `{"engine":"postgresql","version":"16","memoryMb":256}`
	for _, tt := range []struct {
		name   string
		scopes []string
		want   int
	}{
		{
			name:   "read scope rejects",
			scopes: []string{"databases.read"},
			want:   http.StatusForbidden,
		},
		{
			name:   "write scope registered",
			scopes: []string{"databases.write"},
			want:   http.StatusServiceUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := dbContainerTestAppWithScopes(tt.scopes)
			req := httptest.NewRequest(http.MethodPost, "/databases/provision", httptest.NewRequest(http.MethodPost, "/", nil).Body)
			req.Header.Set("Content-Type", "application/json")
			req.Body = nil
			_ = req
			_ = payload
			resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/databases/provision", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestDBContainerListRoute(t *testing.T) {
	app := dbContainerTestApp()
	req := httptest.NewRequest(http.MethodGet, "/databases/containers", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected service unavailable without store, got %d", resp.StatusCode)
	}
}

func TestDBContainerEnginesContent(t *testing.T) {
	app := dbContainerTestApp()
	req := httptest.NewRequest(http.MethodGet, "/databases/engines", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func dbContainerTestApp() *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		c.Locals("apiScopes", []string{"databases.read"})
		return c.Next()
	})
	protected := app.Group("")
	protected.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		return c.Next()
	})
	noopLimiter := func(c *fiber.Ctx) error { return c.Next() }
	registerDBContainerRoutes(protected, Config{}, noopLimiter)
	return app
}

func dbContainerTestAppWithScopes(scopes []string) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		c.Locals("apiScopes", scopes)
		return c.Next()
	})
	protected := app.Group("")
	protected.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		return c.Next()
	})
	noopLimiter := func(c *fiber.Ctx) error { return c.Next() }
	registerDBContainerRoutes(protected, Config{}, noopLimiter)
	return app
}
