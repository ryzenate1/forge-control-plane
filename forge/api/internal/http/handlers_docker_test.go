package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func TestDockerRoutesRequireStoreAndDaemon(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", adminAuth)
	registerDockerRoutes(protected, Config{}, noop, noop)

	type ep struct{ method, path string }
	endpoints := []ep{
		{"GET", "/api/v1/docker/containers"},
		{"POST", "/api/v1/docker/containers"},
		{"GET", "/api/v1/docker/images"},
		{"POST", "/api/v1/docker/images/pull"},
		{"GET", "/api/v1/docker/networks"},
		{"POST", "/api/v1/docker/networks"},
		{"GET", "/api/v1/docker/volumes"},
		{"POST", "/api/v1/docker/volumes"},
	}

	for _, e := range endpoints {
		req := httptest.NewRequest(e.method, e.path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s: %v", e.method, e.path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: expected 404 (nil store/daemon), got %d", e.method, e.path, resp.StatusCode)
		}
	}
}

func TestDockerRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	userAuth := func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Sub: "user-1", Role: "user"})
		return c.Next()
	}
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", userAuth)
	registerDockerRoutes(protected, Config{}, noop, noop)

	type ep struct{ method, path string }
	endpoints := []ep{
		{"GET", "/api/v1/docker/containers"},
		{"POST", "/api/v1/docker/containers"},
	}

	for _, e := range endpoints {
		req := httptest.NewRequest(e.method, e.path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s: %v", e.method, e.path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: expected 404 (nil store), got %d", e.method, e.path, resp.StatusCode)
		}
	}
}

func TestDockerCreateContainerRejectsInvalidImage(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", adminAuth)
	registerDockerRoutes(protected, Config{Store: &store.Store{}, Daemon: &daemon.Client{}}, noop, noop)

	// Image validation runs before node resolution and before any daemon
	// round-trip: garbage must be a 400 here, not a 502 from Beacon.
	body := `{"image":"::::bad image!!!!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docker/containers?node=22222222-2222-2222-2222-222222222222", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid image reference, got %d", resp.StatusCode)
	}
}
