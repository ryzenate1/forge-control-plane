package http

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"

	"github.com/gofiber/fiber/v2"
)

func requestStatus(t *testing.T, app *fiber.App, method, path string, body []byte) (*nethttp.Response, []byte) {
	t.Helper()
	req, err := nethttp.NewRequest(method, path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, payload
}

func TestMigrationLifecycleRoutesReturnNotImplemented(t *testing.T) {
	service := migrationservice.New(nil, nil, nil, nil, nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/migrations/:id/prepare", prepareMigrationRoute(service))
	app.Post("/migrations/:id/execute", executeMigrationRoute(service))

	for _, path := range []string{"/migrations/migration-1/prepare", "/migrations/migration-1/execute"} {
		res, _ := requestStatus(t, app, nethttp.MethodPost, path, nil)
		if res.StatusCode != nethttp.StatusNotImplemented {
			t.Fatalf("%s returned %d, want 501", path, res.StatusCode)
		}
	}
}

func TestRecoveryExecutionReturnsNotImplemented(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/execute", workloadExecutionNotImplemented("recovery execution"))
	res, _ := requestStatus(t, app, nethttp.MethodPost, "/execute", []byte(`{}`))
	if res.StatusCode != nethttp.StatusNotImplemented {
		t.Fatalf("recovery execution returned %d, want 501", res.StatusCode)
	}
}

func TestEvacuationExecutionRouteValidatesPlanIDAndIsNotStubbed(t *testing.T) {
	planner := evacuationplanner.New(nil, nil)
	migrations := migrationservice.New(nil, nil, planner, nil, nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/evacuations", executeEvacuationRoute(planner, migrations))

	res, _ := requestStatus(t, app, nethttp.MethodPost, "/evacuations", []byte(`{}`))
	if res.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("missing planId returned %d, want 400", res.StatusCode)
	}

	res, _ = requestStatus(t, app, nethttp.MethodPost, "/evacuations", []byte(`{"planId":"plan-1"}`))
	if res.StatusCode == nethttp.StatusNotImplemented {
		t.Fatalf("evacuation execution returned 501 instead of invoking the planner")
	}
	if res.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("storeless planner returned %d, want 400", res.StatusCode)
	}
}

func TestEvacuationExecutionRouteRequiresServices(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/evacuations", executeEvacuationRoute(nil, nil))
	res, _ := requestStatus(t, app, nethttp.MethodPost, "/evacuations", []byte(`{"planId":"plan-1"}`))
	if res.StatusCode != nethttp.StatusServiceUnavailable {
		t.Fatalf("unavailable evacuation services returned %d, want 503", res.StatusCode)
	}
}

func TestLegacyServerTransferEndpointsAreRetired(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/servers/:id/transfer", legacyServerTransferUnavailable)
	app.Post("/servers/:id/transfer/cancel", legacyServerTransferUnavailable)
	app.Post("/remote/servers/:id/transfer/success", legacyServerTransferCallbackUnavailable)
	app.Post("/remote/servers/:id/transfer/failure", legacyServerTransferCallbackUnavailable)

	tests := []struct {
		path       string
		statusCode int
		message    string
	}{
		{"/servers/server-1/transfer", nethttp.StatusNotImplemented, "legacy server transfer endpoints are not implemented"},
		{"/servers/server-1/transfer/cancel", nethttp.StatusNotImplemented, "legacy server transfer endpoints are not implemented"},
		{"/remote/servers/server-1/transfer/success", nethttp.StatusGone, "legacy server transfer callbacks have been retired"},
		{"/remote/servers/server-1/transfer/failure", nethttp.StatusGone, "legacy server transfer callbacks have been retired"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			res, body := requestStatus(t, app, nethttp.MethodPost, tt.path, []byte(`{"targetNodeId":"node-2"}`))
			if res.StatusCode != tt.statusCode {
				t.Fatalf("%s returned %d, want %d", tt.path, res.StatusCode, tt.statusCode)
			}
			if !strings.Contains(string(body), tt.message) {
				t.Fatalf("%s error response did not explain deprecation: %s", tt.path, body)
			}
		})
	}
}

func TestPluginLifecycleHandlersReturnNotImplementedWithoutRuntime(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		handler fiber.Handler
	}{
		{name: "install", method: nethttp.MethodPost, handler: InstallPlugin(Config{}, "")},
		{name: "update", method: nethttp.MethodPatch, handler: UpdatePlugin(Config{})},
		{name: "enable", method: nethttp.MethodPost, handler: EnablePlugin(Config{})},
		{name: "disable", method: nethttp.MethodPost, handler: DisablePlugin(Config{})},
		{name: "uninstall", method: nethttp.MethodPost, handler: UninstallPlugin(Config{}, "")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Add(tt.method, "/plugins/:id", tt.handler)
			res, body := requestStatus(t, app, tt.method, "/plugins/plugin-1", []byte(`{}`))
			// install handler still returns 501, the rest return 503 (no store)
			if tt.name == "install" {
				if res.StatusCode != nethttp.StatusNotImplemented {
					t.Fatalf("%s returned %d, want 501", tt.name, res.StatusCode)
				}
				if !strings.Contains(string(body), "plugin runtime is not available") || !strings.Contains(string(body), tt.name) {
					t.Fatalf("%s did not explain its runtime limitation: %s", tt.name, body)
				}
			} else {
				if res.StatusCode != nethttp.StatusServiceUnavailable && res.StatusCode != nethttp.StatusNotImplemented {
					t.Fatalf("%s returned %d, want 501 or 503", tt.name, res.StatusCode)
				}
			}
		})
	}
}

func TestMailTestReturnsServiceUnavailable(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/admin/settings/mail/test", mailTestUnavailable)

	res, payload := requestStatus(t, app, nethttp.MethodPost, "/admin/settings/mail/test", nil)
	if res.StatusCode != nethttp.StatusServiceUnavailable {
		t.Fatalf("mail test returned %d, want 503", res.StatusCode)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if sent, ok := body["sent"].(bool); !ok || sent {
		t.Fatalf("mail test response must explicitly report sent=false: %s", payload)
	}
}

func TestProductionPasswordResetRequestIsUniformlyUnavailable(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	v1 := app.Group("/api/v1")
	registerPasswordResetRoutes(v1, Config{}, func(c *fiber.Ctx) error { return c.Next() })

	var first []byte
	for index, email := range []string{"known@example.com", "unknown@example.com"} {
		res, payload := requestStatus(t, app, nethttp.MethodPost, "/api/v1/auth/password/email", []byte(`{"email":"`+email+`"}`))
		if res.StatusCode != nethttp.StatusServiceUnavailable {
			t.Fatalf("password reset for %s returned %d, want 503", email, res.StatusCode)
		}
		if index == 0 {
			first = payload
			continue
		}
		if !bytes.Equal(first, payload) {
			t.Fatalf("production responses differ by email: %q != %q", first, payload)
		}
	}
}

func TestRealtimeRoutesReturnServiceUnavailableBeforeUpgrade(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "test-secret"})
	for _, path := range []string{
		"/api/v1/servers/server-1/ws/stats",
		"/api/v1/servers/server-1/ws/logs",
		"/api/v1/servers/server-1/ws/console",
	} {
		res, payload := requestStatus(t, app, nethttp.MethodGet, path, nil)
		if res.StatusCode != nethttp.StatusServiceUnavailable {
			t.Fatalf("%s returned %d (%s), want 503", path, res.StatusCode, payload)
		}
	}
}

func TestMigrationAndReservationRoutesAreUnique(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "test-secret"})
	counts := map[string]int{}
	for _, route := range app.GetRoutes() {
		counts[route.Method+" "+route.Path]++
	}

	for _, route := range []string{
		"GET /api/v1/migrations",
		"POST /api/v1/migrations",
		"GET /api/v1/migrations/:id",
		"PATCH /api/v1/migrations/:id/cancel",
		"POST /api/v1/migrations/:id/cancel",
		"POST /api/v1/migrations/:id/prepare",
		"POST /api/v1/migrations/:id/execute",
		"POST /api/v1/evacuations",
		"POST /api/v1/recovery",
		"GET /api/v1/reservations",
		"POST /api/v1/reservations",
		"GET /api/v1/reservations/:id",
		"POST /api/v1/reservations/:id/cancel",
		"POST /api/v1/reservations/:id/confirm",
	} {
		if counts[route] != 1 {
			t.Errorf("route %s registered %d times, want 1", route, counts[route])
		}
	}
}
