package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gamepanel/forge/internal/services/health"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func TestHealth(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second})
	req, err := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var report health.HealthReport
	if err := json.NewDecoder(res.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Status != health.StatusOK || !report.OK || report.Service != "api" {
		t.Fatalf("unexpected fallback health report: %+v", report)
	}
	if report.Checks == nil || len(report.Checks) != 0 {
		t.Fatalf("expected an empty checks array, got %+v", report.Checks)
	}
	if report.CheckedAt.IsZero() {
		t.Fatal("expected fallback report timestamp")
	}
}

func TestProductionHealthContracts(t *testing.T) {
	critical := health.NewService("test")
	critical.AddCheck(health.NewDatabaseCheck(func(_ context.Context) error {
		return errors.New("database unavailable")
	}, nil))
	app := NewServer(Config{ReadTimeout: time.Second, HealthService: critical})

	readyReq, err := http.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	readyRes, err := app.Test(readyReq)
	if err != nil {
		t.Fatal(err)
	}
	defer readyRes.Body.Close()
	if readyRes.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected critical dependency failure to return 503, got %d", readyRes.StatusCode)
	}

	liveReq, err := http.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	liveRes, err := app.Test(liveReq)
	if err != nil {
		t.Fatal(err)
	}
	defer liveRes.Body.Close()
	if liveRes.StatusCode != http.StatusOK {
		t.Fatalf("expected liveness to remain 200, got %d", liveRes.StatusCode)
	}

	diagnosticReq, err := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticRes, err := app.Test(diagnosticReq)
	if err != nil {
		t.Fatal(err)
	}
	defer diagnosticRes.Body.Close()
	if diagnosticRes.StatusCode != http.StatusOK {
		t.Fatalf("expected diagnostics to return 200, got %d", diagnosticRes.StatusCode)
	}
	var diagnostic health.HealthReport
	if err := json.NewDecoder(diagnosticRes.Body).Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic.Status != health.StatusFailed {
		t.Fatalf("expected diagnostic status failed, got %q", diagnostic.Status)
	}
	var databaseCritical bool
	for _, check := range diagnostic.Checks {
		if check.Name == "database" {
			databaseCritical = check.Critical
			break
		}
	}
	if !databaseCritical {
		t.Fatalf("expected failed database check to be marked critical: %+v", diagnostic.Checks)
	}

	nonCritical := health.NewService("test")
	nonCritical.AddCheck(health.NewDaemonCheck(func(_ context.Context) (int, int, int, map[string]any, error) {
		return 0, 0, 0, nil, errors.New("node lookup unavailable")
	}))
	app = NewServer(Config{ReadTimeout: time.Second, HealthService: nonCritical})
	readyRes, err = app.Test(readyReq)
	if err != nil {
		t.Fatal(err)
	}
	defer readyRes.Body.Close()
	if readyRes.StatusCode != http.StatusOK {
		t.Fatalf("expected diagnostic-only failure to remain ready, got %d", readyRes.StatusCode)
	}
}

func TestMetrics(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	req, err := http.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "game_panel_api_uptime_seconds") {
		t.Fatalf("expected api uptime metric, got %s", body)
	}
	if !strings.Contains(string(body), "game_panel_api_up 1") {
		t.Fatalf("expected api availability metric, got %s", body)
	}
}

func TestPowerRejectsInvalidSignal(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	token, err := issueToken("secret", structUser())
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "/api/v1/servers/demo/power", strings.NewReader(`{"signal":"explode"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 503 without an authentication store, got %d: %s", res.StatusCode, body)
	}
}

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	req, err := http.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 401, got %d: %s", res.StatusCode, body)
	}
}

func TestRequireAdminScope(t *testing.T) {
	app := fiber.New()
	app.Get("/allowed", func(c *fiber.Ctx) error {
		c.Locals("apiScopes", []string{"nodes.read"})
		return c.Next()
	}, requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	app.Get("/denied", func(c *fiber.Ctx) error {
		c.Locals("apiScopes", []string{"users.read"})
		return c.Next()
	}, requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	allowedReq, err := http.NewRequest(http.MethodGet, "/allowed", nil)
	if err != nil {
		t.Fatal(err)
	}
	allowedRes, err := app.Test(allowedReq)
	if err != nil {
		t.Fatal(err)
	}
	defer allowedRes.Body.Close()
	if allowedRes.StatusCode != http.StatusOK {
		t.Fatalf("expected allowed status 200, got %d", allowedRes.StatusCode)
	}

	deniedReq, err := http.NewRequest(http.MethodGet, "/denied", nil)
	if err != nil {
		t.Fatal(err)
	}
	deniedRes, err := app.Test(deniedReq)
	if err != nil {
		t.Fatal(err)
	}
	defer deniedRes.Body.Close()
	if deniedRes.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(deniedRes.Body)
		t.Fatalf("expected denied status 403, got %d: %s", deniedRes.StatusCode, body)
	}
}

func TestLoginRequiresStore(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"admin123"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 503, got %d: %s", res.StatusCode, body)
	}
}

func TestMissingStoreReturns503(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	token, err := issueToken("secret", structUser())
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 503, got %d: %s", res.StatusCode, body)
	}
}

func TestAuthMeReturnsClaims(t *testing.T) {
	app := NewServer(Config{ReadTimeout: time.Second, AuthSecret: "secret"})
	token, err := issueToken("secret", structUser())
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 503 without an authentication store, got %d: %s", res.StatusCode, body)
	}
}

func structUser() store.User {
	return store.User{ID: "user-1", Email: "admin@example.com", Role: "admin", SessionVersion: 1}
}
