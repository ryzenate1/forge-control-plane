package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDebugPasswordResetRouting(t *testing.T) {
	cfg := Config{
		AuthSecret: "test-secret-for-routing-debug-1234567890",
		AppEnv:     "development",
		Store:      nil,
	}
	app := NewServer(cfg)
	// Dump POST bucket order around our path (Fiber matches first-in-bucket).
	for _, r := range app.GetRoutes() {
		if r.Method == "POST" && strings.HasPrefix(r.Path, "/api/v1/auth/") {
			fmt.Printf("AUTH-POST %s handlers=%d\n", r.Path, len(r.Handlers))
		}
	}
	for _, r := range app.GetRoutes() {
		if r.Method == "POST" && (strings.Contains(r.Path, ":") || strings.Contains(r.Path, "*")) {
			fmt.Printf("PARAM-ROUTE %s %s\n", r.Method, r.Path)
		}
	}
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/auth/password/email"},
		{"POST", "/api/v1/auth/password/reset"},
		{"POST", "/api/v1/auth/login"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"email":"x@y.z"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://localhost:3000")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		fmt.Printf("REQ %s %s -> %d %s\n", tc.method, tc.path, res.StatusCode, strings.TrimSpace(string(b)))
	}
	_ = fiber.StatusOK
	_ = http.StatusOK
}
