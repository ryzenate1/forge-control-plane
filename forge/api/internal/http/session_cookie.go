package http

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	SessionCookieName = "__Host-forge_session"
	CSRFCookieName    = "__Host-forge_csrf"
)

type SessionCookieConfig struct {
	Secure   bool
	SameSite http.SameSite
}

func LoadSessionCookieConfig() SessionCookieConfig {
	secure := true
	if v := strings.ToLower(os.Getenv("SESSION_COOKIE_SECURE")); v == "false" || v == "0" {
		secure = false
	}
	sameSite := http.SameSiteLaxMode
	if v := strings.ToLower(os.Getenv("SESSION_COOKIE_SAME_SITE")); v == "strict" {
		sameSite = http.SameSiteStrictMode
	} else if v == "none" {
		sameSite = http.SameSiteNoneMode
	}
	return SessionCookieConfig{Secure: secure, SameSite: sameSite}
}

func ValidateSessionCookieConfig(cfg SessionCookieConfig, appEnv string) error {
	if appEnv == "production" && !cfg.Secure {
		return ErrInsecureCookieConfig
	}
	return nil
}

func generateCSRFToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

var ErrInsecureCookieConfig = errors.New("SESSION_COOKIE_SECURE must be true in production")

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, cfg SessionCookieConfig) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	}
	http.SetCookie(w, cookie)
}

func setCSRFCookie(w http.ResponseWriter, token string, expires time.Time, cfg SessionCookieConfig) {
	cookie := &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter, cfg SessionCookieConfig) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, cookie)
}

func clearCSRFCookie(w http.ResponseWriter, cfg SessionCookieConfig) {
	cookie := &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, cookie)
}

func getCSRFTokenFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}

func getSessionTokenFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}

// exchangeCodeEntry holds a session token that can be claimed once via a short-lived code.
type exchangeCodeEntry struct {
	token     string
	csrfToken string
	expiresAt time.Time
}

// exchangeCodeStore is an in-memory store for single-use session exchange codes.
// Codes are short-lived (60s) and can be claimed exactly once.
type exchangeCodeStore struct {
	mu   sync.Mutex
	data map[string]*exchangeCodeEntry
}

var globalExchangeCodes = &exchangeCodeStore{data: make(map[string]*exchangeCodeEntry)}

func (s *exchangeCodeStore) issue(token, csrfToken string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := hex.EncodeToString(raw)
	s.mu.Lock()
	s.data[code] = &exchangeCodeEntry{token: token, csrfToken: csrfToken, expiresAt: time.Now().Add(60 * time.Second)}
	s.mu.Unlock()
	return code, nil
}

func (s *exchangeCodeStore) claim(code string) (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[code]
	if !ok {
		return "", "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.data, code)
		return "", "", false
	}
	delete(s.data, code)
	return entry.token, entry.csrfToken, true
}

func (s *exchangeCodeStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for code, entry := range s.data {
		if now.After(entry.expiresAt) {
			delete(s.data, code)
		}
	}
}

// ExchangeCodeHandler exchanges a single-use code for the session token and
// sets HttpOnly cookies directly, so the client never receives the raw JWT.
// Used by social auth callbacks to avoid placing the session token in the redirect URL.
func ExchangeCodeHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Code string `json:"code"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Code == "" {
			return fiber.NewError(fiber.StatusBadRequest, "code is required")
		}
		token, csrfToken, ok := globalExchangeCodes.claim(req.Code)
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "invalid or expired exchange code")
		}
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)
		return c.JSON(fiber.Map{"ok": true})
	}
}

// issueExchangeCode creates a single-use code for the given session token.
func issueExchangeCode(token, csrfToken string) (string, error) {
	return globalExchangeCodes.issue(token, csrfToken)
}
