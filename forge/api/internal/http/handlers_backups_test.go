package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestBackupRoutes_NilService(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", adminAuth)
	registerBackupRoutes(protected, Config{Store: nil}, nil, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/servers/srv-1/backups/policies", ""},
		{http.MethodPost, "/api/v1/servers/srv-1/backups/policies", `{}`},
		{http.MethodDelete, "/api/v1/servers/srv-1/backups/policies/pol-1", ""},
		{http.MethodPost, "/api/v1/servers/srv-1/backups", `{}`},
		{http.MethodGet, "/api/v1/backup/providers", ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req = httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
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

func TestBackupRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	registerBackupRoutes(protected, Config{Store: nil}, nil, noop)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/srv-1/backups/policies", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nil service, got %d", resp.StatusCode)
	}
}
