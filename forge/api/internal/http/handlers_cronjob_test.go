package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func TestCronJobs_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	protected := app.Group("/api/v1", adminAuth)
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerCronJobRoutes(protected, Config{Store: nil}, nil, noop)

	tests := []struct {
		name string
		method string
		path string
		body string
	}{
		{"GET /cron-jobs", http.MethodGet, "/api/v1/cron-jobs", ""},
		{"POST /cron-jobs", http.MethodPost, "/api/v1/cron-jobs", `{}`},
		{"GET /cron-jobs/:id", http.MethodGet, "/api/v1/cron-jobs/job-1", ""},
		{"PUT /cron-jobs/:id", http.MethodPut, "/api/v1/cron-jobs/job-1", `{}`},
		{"DELETE /cron-jobs/:id", http.MethodDelete, "/api/v1/cron-jobs/job-1", ""},
		{"POST /cron-jobs/:id/toggle", http.MethodPost, "/api/v1/cron-jobs/job-1/toggle", ""},
		{"GET /cron-jobs/:id/executions", http.MethodGet, "/api/v1/cron-jobs/job-1/executions", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 for nil store, got %d", resp.StatusCode)
			}
		})
	}
}

func TestCronJobs_List(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	protected := app.Group("/api/v1", adminAuth)
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerCronJobRoutes(protected, Config{Store: nil}, nil, noop)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cron-jobs", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store, got %d", resp.StatusCode)
	}
}

func TestCronJobs_Create(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	protected := app.Group("/api/v1", adminAuth)
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerCronJobRoutes(protected, Config{Store: &store.Store{}}, nil, noop)

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(`{not-json}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(""))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		body := `{"name":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing fields, got %d", resp.StatusCode)
		}
	})

	t.Run("command too long", func(t *testing.T) {
		body := `{"name":"test","schedule":"*/5 * * * *","command":"` + strings.Repeat("a", 4097) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for long command, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid cron schedule", func(t *testing.T) {
		body := `{"name":"test","schedule":"* * * * *","command":"echo hello"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid cron schedule, got %d", resp.StatusCode)
		}
	})

	t.Run("not enough cron fields", func(t *testing.T) {
		body := `{"name":"test","schedule":"* * * *","command":"echo hello"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid cron fields, got %d", resp.StatusCode)
		}
	})
}

func TestCronJobs_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	protected := app.Group("/api/v1")
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerCronJobRoutes(protected, Config{Store: nil}, nil, noop)

	endpoints := []struct {
		method string
		path string
		body string
	}{
		{http.MethodGet, "/api/v1/cron-jobs", ""},
		{http.MethodPost, "/api/v1/cron-jobs", `{}`},
		{http.MethodGet, "/api/v1/cron-jobs/job-1", ""},
		{http.MethodPut, "/api/v1/cron-jobs/job-1", `{}`},
		{http.MethodDelete, "/api/v1/cron-jobs/job-1", ""},
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
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
			}
		})
	}
}
