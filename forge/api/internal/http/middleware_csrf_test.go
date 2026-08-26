package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// csrfTestApp builds a bare fiber app with the CSRF middleware enabled for
// cookie-session-authenticated requests, mirroring how the real server wires
// csrfMiddleware after authMiddleware sets the authSource local. The helper
// pins SESSION_COOKIE_SECURE=false so the global session-cookie lookup agrees
// with the insecure cookie names the tests send (matching
// SessionCookieConfig{Secure: false}); without this the lookup defaults to
// the __Host- names and every request fails before reaching the CSRF checks.
func csrfTestApp(t *testing.T, secret string, cfg SessionCookieConfig, panelOrigin string) *fiber.App {
	t.Helper()
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("authSource", authSourceCookieSession)
		if panelOrigin != "" {
			c.Locals("panelOrigin", panelOrigin)
		}
		return c.Next()
	}, csrfMiddleware(secret, cfg))
	app.Post("/mutate", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Put("/mutate", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Patch("/mutate", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Delete("/mutate", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/safe", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Head("/safe", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Options("/safe", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	return app
}

const csrfTestSecret = "test-csrf-binding-secret"

// csrfSessionCookies builds the Cookie header for a session plus its bound
// CSRF token (insecure names, matching SessionCookieConfig{Secure: false}).
func csrfSessionCookies(session, csrf string) string {
	cookies := "forge_session=" + session
	if csrf != "" {
		cookies += "; forge_csrf=" + csrf
	}
	return cookies
}

func csrfBoundToken(session string) string {
	return deriveSessionCSRFToken(csrfTestSecret, session)
}

func csrfTestRequest(t *testing.T, app *fiber.App, method, path, cookie, header, origin, fetchSite string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if header != "" {
		req.Header.Set("X-CSRF-Token", header)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return res, string(body)
}

func TestCSRFMiddleware_RejectsMutationWithoutCookie(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			res, body := csrfTestRequest(t, app, method, "/mutate", "", "", "", "")
			if res.StatusCode != http.StatusForbidden {
				t.Fatalf("%s without CSRF cookie returned %d, want 403", method, res.StatusCode)
			}
			if !strings.Contains(body, "missing session") {
				t.Fatalf("expected missing-session error, got: %s", body)
			}
		})
	}
}

func TestCSRFMiddleware_RejectsMutationWithoutHeader(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", csrfSessionCookies("sess-1", csrfBoundToken("sess-1")), "", "", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("POST with cookie but no header returned %d, want 403", res.StatusCode)
	}
	if !strings.Contains(body, "missing X-CSRF-Token header") {
		t.Fatalf("expected missing-header error, got: %s", body)
	}
}

func TestCSRFMiddleware_RejectsMismatchedToken(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", csrfSessionCookies("sess-1", "abc123"), "different-token", "", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("POST with mismatched tokens returned %d, want 403", res.StatusCode)
	}
	if !strings.Contains(body, "invalid CSRF token") {
		t.Fatalf("expected invalid-token error, got: %s", body)
	}
}

// TestCSRFMiddleware_RejectsUnboundToken pins the session binding: a
// cookie/header pair that agrees with itself but was not derived from the
// presented session must be rejected, otherwise a token fixed or stolen for
// another session would validate.
func TestCSRFMiddleware_RejectsUnboundToken(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", csrfSessionCookies("sess-1", csrfBoundToken("other-session")), csrfBoundToken("other-session"), "", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("POST with foreign-session token returned %d, want 403", res.StatusCode)
	}
	if !strings.Contains(body, "invalid CSRF token") {
		t.Fatalf("expected invalid-token error, got: %s", body)
	}
}

func TestCSRFMiddleware_PassesWithMatchingToken(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			res, body := csrfTestRequest(t, app, method, "/mutate", csrfSessionCookies("sess-1", csrfBoundToken("sess-1")), csrfBoundToken("sess-1"), "", "")
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("%s with matching tokens returned %d, want 204: %s", method, res.StatusCode, body)
			}
		})
	}
}

func TestCSRFMiddleware_ExemptsSafeMethods(t *testing.T) {
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			res, body := csrfTestRequest(t, app, method, "/safe", "", "", "", "")
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("%s without any token returned %d, want 204: %s", method, res.StatusCode, body)
			}
		})
	}
}

func TestCSRFMiddleware_SkipsNonCookieSessionAuth(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("authSource", authSourceAPIKey)
		return c.Next()
	}, csrfMiddleware(csrfTestSecret, SessionCookieConfig{Secure: false}))
	app.Post("/mutate", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", "", "", "", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("API-key-authenticated POST without CSRF token returned %d, want 204: %s", res.StatusCode, body)
	}
}

func TestCSRFMiddleware_OriginEnforcement(t *testing.T) {
	const panelOrigin = "https://panel.example.com"
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, panelOrigin)

	tests := []struct {
		name      string
		origin    string
		fetchSite string
		wantCode  int
		wantBody  string
	}{
		{name: "missing origin and fetch-site", wantCode: http.StatusForbidden, wantBody: "missing Origin header"},
		{name: "cross-origin", origin: "https://evil.example.com", wantCode: http.StatusForbidden, wantBody: "invalid Origin"},
		{name: "matching origin", origin: panelOrigin, wantCode: http.StatusNoContent},
		{name: "fetch-site fallback", fetchSite: "same-origin", wantCode: http.StatusNoContent},
		{name: "fetch-site same-site", fetchSite: "same-site", wantCode: http.StatusNoContent},
		{name: "fetch-site strict-same-origin", fetchSite: "strict-same-origin", wantCode: http.StatusNoContent},
		// Regression: the fallback accepted any non-empty Sec-Fetch-Site, so a
		// request that announced itself as cross-site passed the origin check.
		{name: "fetch-site cross-site", fetchSite: "cross-site", wantCode: http.StatusForbidden, wantBody: "missing Origin header"},
		{name: "fetch-site none", fetchSite: "none", wantCode: http.StatusForbidden, wantBody: "missing Origin header"},
		{name: "fetch-site garbage", fetchSite: "not-a-real-value", wantCode: http.StatusForbidden, wantBody: "missing Origin header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", csrfSessionCookies("sess-1", csrfBoundToken("sess-1")), csrfBoundToken("sess-1"), tt.origin, tt.fetchSite)
			if res.StatusCode != tt.wantCode {
				t.Fatalf("POST returned %d, want %d: %s", res.StatusCode, tt.wantCode, body)
			}
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Fatalf("expected error %q, got: %s", tt.wantBody, body)
			}
		})
	}
}

func TestGenerateCSRFToken_FormatAndUniqueness(t *testing.T) {
	a, err := GenerateCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != CSRFTokenLength*2 {
		t.Fatalf("token length = %d, want %d hex chars", len(a), CSRFTokenLength*2)
	}
	if a == b {
		t.Fatal("two generated tokens must differ")
	}
}

func TestSetCSRFCookie_AttributesAndExpiry(t *testing.T) {
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	t.Setenv("SESSION_COOKIE_SAME_SITE", "strict")
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/issue", func(c *fiber.Ctx) error {
		SetCSRFCookie(c, "abc123")
		return c.SendStatus(fiber.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/issue", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	setCookie := res.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "forge_csrf=abc123") {
		t.Fatalf("expected CSRF cookie in Set-Cookie, got: %s", setCookie)
	}
	if strings.Contains(setCookie, "HttpOnly") {
		t.Fatal("CSRF cookie must not be HttpOnly so the web client can read it")
	}
	if !strings.Contains(setCookie, "SameSite=Strict") {
		t.Fatalf("expected SameSite=Strict, got: %s", setCookie)
	}
	if !strings.Contains(strings.ToLower(setCookie), "expires=") {
		t.Fatalf("expected an Expires attribute (~CSRFTokenExpiry), got: %s", setCookie)
	}
}

func TestGetCSRFTokenHandler_SetsCookie(t *testing.T) {
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/csrf", GetCSRFTokenHandler(Config{}))

	req := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}
	setCookie := res.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "forge_csrf=") {
		t.Fatalf("expected CSRF cookie issuance, got: %s", setCookie)
	}
}

func TestCSRFMiddleware_SecureCookieName(t *testing.T) {
	if got := secureCookieName(CSRFCookieName, true); got != "__Host-forge_csrf" {
		t.Fatalf("secure cookie name = %q, want __Host-forge_csrf", got)
	}
	if got := secureCookieName(CSRFCookieName, false); got != "forge_csrf" {
		t.Fatalf("insecure cookie name = %q, want forge_csrf", got)
	}
}

func TestCSRFMiddleware_DoubleSubmitRoundTrip(t *testing.T) {
	token := csrfBoundToken("round-trip-session")
	if token == "" {
		t.Fatal("bound token must be non-empty")
	}
	app := csrfTestApp(t, csrfTestSecret, SessionCookieConfig{Secure: false}, "")
	res, body := csrfTestRequest(t, app, http.MethodPost, "/mutate", csrfSessionCookies("round-trip-session", token), token, "", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("double-submit with freshly derived token returned %d, want 204: %s", res.StatusCode, body)
	}
	if token == "" || time.Now().IsZero() {
		t.Fatal("token should be non-empty")
	}
}
