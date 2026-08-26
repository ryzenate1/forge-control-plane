package http

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestGetWebSocketAllowedOrigins_MergesPanelAndCORS(t *testing.T) {
	origCORS := os.Getenv("API_CORS_ALLOWED_ORIGINS")
	origWS := os.Getenv("API_WS_ALLOWED_ORIGINS")
	origPanel := os.Getenv("PANEL_URL")
	t.Cleanup(func() {
		os.Setenv("API_CORS_ALLOWED_ORIGINS", origCORS)
		os.Setenv("API_WS_ALLOWED_ORIGINS", origWS)
		os.Setenv("PANEL_URL", origPanel)
	})

	os.Setenv("API_CORS_ALLOWED_ORIGINS", "https://cors.example.com")
	os.Setenv("PANEL_URL", "https://panel.example.com")
	os.Setenv("API_WS_ALLOWED_ORIGINS", "")

	cfg := Config{
		AppEnv:   "development",
		PanelURL: "https://panel.example.com",
		CORSConfig: CORSConfig{
			AllowedOrigins: []string{"https://cors.example.com"},
		},
	}
	allowed := getWebSocketAllowedOrigins(cfg)
	assertContains := func(want string) {
		for _, a := range allowed {
			if strings.EqualFold(a, want) {
				return
			}
		}
		t.Fatalf("allowed origins %v does not contain %q", allowed, want)
	}
	assertContains("https://panel.example.com")
	assertContains("https://cors.example.com")

	// Explicit WS env should also be included
	os.Setenv("API_WS_ALLOWED_ORIGINS", "https://ws.example.com")
	allowed = getWebSocketAllowedOrigins(cfg)
	assertContains("https://ws.example.com")
	// Panel URL still present even with explicit WS env (merged)
	assertContains("https://panel.example.com")

	// Wildcard stripped in production
	os.Setenv("API_WS_ALLOWED_ORIGINS", "*, https://panel.example.com")
	cfg.AppEnv = "production"
	allowed = getWebSocketAllowedOrigins(cfg)
	for _, a := range allowed {
		if a == "*" {
			t.Fatal("wildcard should be stripped in production")
		}
	}
	assertContains("https://panel.example.com")
}

// TestGetWebSocketAllowedOrigins_NeverEmpty covers the case the wildcard test
// above cannot reach: production with nothing configured.
//
// An empty allow-list is not a closed door. gofiber/contrib/websocket turns an
// empty Config.Origins into []string{"*"} and then accepts every origin, so
// returning zero entries here would open exactly the cross-origin WS hole this
// builder exists to prevent — and it would do so precisely when the operator
// configured no origins, and also when they set API_WS_ALLOWED_ORIGINS="*" in
// production, since that entry is stripped and leaves the list empty.
func TestGetWebSocketAllowedOrigins_NeverEmpty(t *testing.T) {
	t.Setenv("API_WS_ALLOWED_ORIGINS", "")
	t.Setenv("API_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("PANEL_URL", "")
	t.Setenv("APP_URL", "")

	prod := Config{AppEnv: "production"}
	allowed := getWebSocketAllowedOrigins(prod)
	if len(allowed) == 0 {
		t.Fatal("empty allow-list lets the websocket middleware default to \"*\"; must return a deny sentinel")
	}
	if allowed[0] == "*" {
		t.Fatalf("first entry must not be the wildcard, got %v", allowed)
	}
	for _, a := range allowed {
		// A browser-sent Origin is scheme://host[:port]; `null` is sent by
		// sandboxed iframes and file:// pages and so must not be the sentinel.
		if strings.Contains(a, "//") || strings.EqualFold(a, "null") {
			t.Fatalf("deny sentinel %q could match a real Origin header", a)
		}
	}

	// A lone wildcard in production is stripped, which is what makes the list
	// empty — the sentinel has to hold that case too.
	t.Setenv("API_WS_ALLOWED_ORIGINS", "*")
	allowed = getWebSocketAllowedOrigins(prod)
	if len(allowed) == 0 || allowed[0] == "*" {
		t.Fatalf("stripped wildcard must leave a deny sentinel, got %v", allowed)
	}

	// Once a real origin is configured the sentinel must not linger and start
	// shadowing it as Origins[0].
	t.Setenv("API_WS_ALLOWED_ORIGINS", "https://panel.example.com")
	allowed = getWebSocketAllowedOrigins(prod)
	for _, a := range allowed {
		if a == wsOriginDenyAll {
			t.Fatalf("deny sentinel must not appear alongside real origins: %v", allowed)
		}
	}
}

// TestWebSocketOriginEnforcement_ViaFiberWSConfig pins the surviving origin
// guard. NOTE: the validateWebSocketOrigin / wsOriginMiddleware /
// wsUpgraderOrigins helpers were removed — per-request origin decisions are no
// longer made in a bespoke middleware. Production now enforces the allowlist by
// passing getWebSocketAllowedOrigins(cfg) into every fiberws route config
// (server.go realtime mounts, handlers_files.go), which the websocket
// middleware consults before upgrading. This test verifies that wiring by
// source scan and keeps the allowlist-builder contract covered above.
func TestWebSocketOriginEnforcement_ViaFiberWSConfig(t *testing.T) {
	src := readHTTPFile(t, "server.go")
	if !strings.Contains(src, "Origins: getWebSocketAllowedOrigins(cfg)") {
		t.Fatal("server.go websocket routes must pass the WS origin allowlist to fiberws.Config")
	}
	if strings.Count(src, "Origins: getWebSocketAllowedOrigins(cfg)") < 4 {
		t.Fatal("expected every realtime ws mount to carry the origins allowlist")
	}
	filesSrc := readHTTPFile(t, "handlers_files.go")
	if !strings.Contains(filesSrc, "Origins: getWebSocketAllowedOrigins(cfg)") {
		t.Fatal("handlers_files.go websocket mount must pass the WS origin allowlist")
	}

	// The allowlist builder itself still merges panel + CORS + explicit env and
	// strips the wildcard in production (full behavior covered by
	// TestGetWebSocketAllowedOrigins_MergesPanelAndCORS).
	t.Setenv("PANEL_URL", "https://panel.example.com")
	t.Setenv("API_WS_ALLOWED_ORIGINS", "https://ws.example.com")
	allowed := getWebSocketAllowedOrigins(Config{AppEnv: "development", PanelURL: "https://panel.example.com"})
	found := false
	for _, a := range allowed {
		if strings.EqualFold(a, "https://ws.example.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit WS origin missing from allowlist: %v", allowed)
	}
}

// TestSameSiteNone_CookieConfig pins the session-cookie side of the old
// SameSite=None origin test. The origin-check assertions that used
// validateWebSocketOrigin were dropped together with the helper (see above);
// the cookie-mode contract itself is unchanged.
func TestSameSiteNone_StillRequiresOrigin(t *testing.T) {
	os.Setenv("SESSION_COOKIE_SAME_SITE", "none")
	os.Setenv("SESSION_COOKIE_SECURE", "true")
	defer os.Unsetenv("SESSION_COOKIE_SAME_SITE")
	defer os.Unsetenv("SESSION_COOKIE_SECURE")

	sessCfg := LoadSessionCookieConfig()
	if sessCfg.SameSite != http.SameSiteNoneMode {
		t.Fatalf("expected SameSite None, got %v", sessCfg.SameSite)
	}
	if !sessCfg.Secure {
		t.Fatal("SameSite=None cookies must be Secure")
	}
}
