package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCloudRoutes_NilService(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerCloudRoutes(protected, Config{Store: nil}, nil, noop, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/admin/cloud/providers", ""},
		{http.MethodGet, "/admin/cloud/links", ""},
		{http.MethodGet, "/admin/cloud/providers/aws/regions", ""},
		{http.MethodGet, "/admin/cloud/providers/aws/types", ""},
		{http.MethodGet, "/admin/cloud/instances", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404 for nil service, got %d", resp.StatusCode)
			}
		})
	}
}

func TestCloudRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin")
	registerCloudRoutes(protected, Config{Store: nil}, nil, noop, noop)

	req := httptest.NewRequest(http.MethodGet, "/admin/cloud/providers", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nil service, got %d", resp.StatusCode)
	}
}
