package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func adminAuth(c *fiber.Ctx) error {
	c.Locals("user", tokenClaims{Sub: "test-user", Role: "admin"})
	c.Locals("apiScopes", []string{"nodes.read", "nodes.write", "nodes.delete", "servers.read", "servers.write"})
	return c.Next()
}

func TestPostNodes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: nil}, nil, nil, nil, nil, nil, nil, noop, noop)

	body := `{"name":"test-node","region":"us-east","locationId":"loc-1","baseUrl":"https://node.example.com:8080","fqdn":"node.example.com","scheme":"https","daemonListen":8080,"daemonSftp":2022}`
	req := httptest.NewRequest(http.MethodPost, "/admin/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store, got %d", resp.StatusCode)
	}
}

func TestPostNodes_InvalidJSON(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: &store.Store{}}, nil, nil, nil, nil, nil, nil, noop, noop)

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes", strings.NewReader(`{not-json}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestPostNodes_EmptyBody(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: &store.Store{}}, nil, nil, nil, nil, nil, nil, noop, noop)

	req := httptest.NewRequest(http.MethodPost, "/admin/nodes", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestPostNodes_ValidationFailure(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	mockStore := &store.Store{}
	registerAdminRoutes(protected, Config{Store: mockStore}, nil, nil, nil, nil, nil, nil, noop, noop)

	body := `{"name":"","region":"us-east","locationId":"loc-1","scheme":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422 for invalid request, got %d", resp.StatusCode)
	}
}

func TestGetNodes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: nil}, nil, nil, nil, nil, nil, nil, noop, noop)

	req := httptest.NewRequest(http.MethodGet, "/admin/nodes", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store for GET /nodes, got %d", resp.StatusCode)
	}
}

func TestGetNodeByID_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: nil}, nil, nil, nil, nil, nil, nil, noop, noop)

	req := httptest.NewRequest(http.MethodGet, "/admin/nodes/node-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store for GET /nodes/:id, got %d", resp.StatusCode)
	}
}

func TestDeleteNodes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerAdminRoutes(protected, Config{Store: nil}, nil, nil, nil, nil, nil, nil, noop, noop)

	req := httptest.NewRequest(http.MethodDelete, "/admin/nodes/node-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store for DELETE /nodes/:id, got %d", resp.StatusCode)
	}
}
