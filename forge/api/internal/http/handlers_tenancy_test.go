package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestTenancyRoutes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	protected := app.Group("/api/v1", adminAuth)
	registerTenancyRoutes(protected, Config{Store: nil}, nil, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/organizations"},
		{http.MethodPost, "/api/v1/organizations"},
		{http.MethodGet, "/api/v1/organizations/my-org"},
		{http.MethodDelete, "/api/v1/organizations/org-1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404 for nil store, got %d", resp.StatusCode)
			}
		})
	}
}

func tenancyOrgAccessApp(user *tokenClaims) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user", *user)
		}
		return c.Next()
	})
	app.Get("/orgs/:id", tenancyOrgAccess(nil), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app
}

func TestTenancyOrgAccess_MissingSession(t *testing.T) {
	app := tenancyOrgAccessApp(nil)

	req := httptest.NewRequest(http.MethodGet, "/orgs/org-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", resp.StatusCode)
	}
}

func TestTenancyOrgAccess_AdminBypass(t *testing.T) {
	app := tenancyOrgAccessApp(&tokenClaims{Sub: "admin-1", Role: "admin"})

	req := httptest.NewRequest(http.MethodGet, "/orgs/org-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for admin bypass, got %d", resp.StatusCode)
	}
}
