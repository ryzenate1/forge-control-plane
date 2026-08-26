package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	authSourceCookieSession = "cookie-session"
	authSourceOAuth         = "oauth"
	authSourceAPIKey        = "api-key"

	CSRFTokenLength = 32
	CSRFTokenExpiry = 2 * time.Hour
)

// csrfBindingDomain separates the session-bound CSRF derivation from every
// other HMAC use of the auth secret.
const csrfBindingDomain = "forge:csrf:v1\x00"

// deriveSessionCSRFToken binds a CSRF token to the session cookie value with
// a keyed HMAC so a token minted for one session cannot be replayed against
// another. It is deterministic per session and infallible; an empty secret
// yields an empty token and every caller must treat that as "no token".
func deriveSessionCSRFToken(secret, sessionToken string) string {
	if secret == "" || sessionToken == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(csrfBindingDomain))
	mac.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validSessionBoundCSRF reports whether the presented double-submit pair is
// both self-consistent and bound to the given session value. All comparisons
// are constant-time and every failure mode denies.
func validSessionBoundCSRF(secret, sessionToken, cookieToken, headerToken string) bool {
	if secret == "" || sessionToken == "" || cookieToken == "" || headerToken == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
		return false
	}
	expected := deriveSessionCSRFToken(secret, sessionToken)
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(expected)) == 1
}

func csrfMiddleware(secret string, cfg SessionCookieConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := c.Method()
		if method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions {
			return c.Next()
		}

		authSource, _ := c.Locals("authSource").(string)
		if authSource != authSourceCookieSession {
			return c.Next()
		}

		if secret == "" {
			return fiber.NewError(fiber.StatusInternalServerError, "auth secret is not configured")
		}
		sessionToken, ok := getSessionCookie(c)
		if !ok || sessionToken == "" {
			return fiber.NewError(fiber.StatusForbidden, "missing session for CSRF validation")
		}

		csrfCookie := c.Cookies(secureCookieName(CSRFCookieName, cfg.Secure))
		if csrfCookie == "" {
			return fiber.NewError(fiber.StatusForbidden, "missing CSRF cookie")
		}

		csrfHeader := c.Get("X-CSRF-Token")
		if csrfHeader == "" {
			return fiber.NewError(fiber.StatusForbidden, "missing X-CSRF-Token header")
		}

		// Double-submit match plus binding to the session that the cookie
		// carries: a token stolen or fixed for another session is rejected
		// even when cookie and header agree with each other.
		if !validSessionBoundCSRF(secret, sessionToken, csrfCookie, csrfHeader) {
			return fiber.NewError(fiber.StatusForbidden, "invalid CSRF token")
		}

		origin := c.Get("Origin")
		panelOrigin := c.Locals("panelOrigin")
		if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
			if origin == "" {
				// No Origin: fall back to Sec-Fetch-Site, which only vouches
				// for the request when the browser says it came from our own
				// site. Accepting any non-empty value (the previous behaviour)
				// let a request that explicitly announced itself as
				// "cross-site" satisfy the check — see
				// publicMutationOriginCheck below for the same rule.
				switch c.Get("Sec-Fetch-Site") {
				case "same-origin", "same-site", "strict-same-origin":
				default:
					return fiber.NewError(fiber.StatusForbidden, "missing Origin header")
				}
			} else if origin != panelOriginStr {
				return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
			}
		}

		return c.Next()
	}
}

func GenerateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func SetCSRFCookie(c *fiber.Ctx, token string) {
	cfg := LoadSessionCookieConfig()
	sameSite := "Lax"
	if cfg.SameSite == http.SameSiteStrictMode {
		sameSite = "Strict"
	} else if cfg.SameSite == http.SameSiteNoneMode {
		sameSite = "None"
	}
	c.Cookie(&fiber.Cookie{
		Name:     secureCookieName(CSRFCookieName, cfg.Secure),
		Value:    token,
		HTTPOnly: false,
		Secure:   cfg.Secure,
		SameSite: sameSite,
		Expires:  time.Now().Add(CSRFTokenExpiry),
		Path:     "/",
	})
}

func GetCSRFTokenHandler(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Prefer a token bound to the caller's session: the CSRF cookie must
		// echo a value derived from the session cookie, so minting a fresh
		// random token here would immediately fail validation on the next
		// mutation. Without a session (pre-login) there is nothing to bind
		// to; the random token is harmless because login rotates both cookies.
		var token string
		if sessionToken, ok := getSessionCookie(c); ok && sessionToken != "" {
			token = deriveSessionCSRFToken(cfg.AuthSecret, sessionToken)
		}
		if token == "" {
			var err error
			token, err = GenerateCSRFToken()
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to generate CSRF token")
			}
		}

		SetCSRFCookie(c, token)

		return c.SendStatus(fiber.StatusNoContent)
	}
}

func publicMutationOriginCheck(cfg SessionCookieConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := c.Method()
		if method != fiber.MethodPost && method != fiber.MethodPut && method != fiber.MethodPatch && method != fiber.MethodDelete {
			return c.Next()
		}

		origin := c.Get("Origin")
		panelOrigin := c.Locals("panelOrigin")
		if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
			if origin == "" {
				// Reject state-changing requests without Origin when panelOrigin is configured
				fetchSite := c.Get("Sec-Fetch-Site")
				if fetchSite != "same-origin" && fetchSite != "same-site" && fetchSite != "strict-same-origin" {
					return fiber.NewError(fiber.StatusForbidden, "missing Origin header on state-changing request")
				}
			} else if origin != panelOriginStr {
				return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
			}
		}

		contentType := c.Get("Content-Type")
		if method == fiber.MethodPost || method == fiber.MethodPut || method == fiber.MethodPatch {
			if contentType == "" && c.Request().Header.ContentLength() > 0 {
				return fiber.NewError(fiber.StatusForbidden, "missing Content-Type header")
			}
			if contentType != "" && !strings.HasPrefix(contentType, "application/json") && !strings.HasPrefix(contentType, "multipart/form-data") {
				return fiber.NewError(fiber.StatusForbidden, "invalid Content-Type")
			}
		}

		return c.Next()
	}
}
