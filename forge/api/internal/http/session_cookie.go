package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
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
	if cfg.SameSite == http.SameSiteNoneMode && !cfg.Secure {
		return errors.New("SameSite=None requires Secure=true")
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

func secureCookieName(baseName string, secure bool) string {
	if secure {
		return baseName
	}
	return strings.TrimPrefix(baseName, "__Host-")
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, cfg SessionCookieConfig) {
	cookie := &http.Cookie{
		Name:     secureCookieName(SessionCookieName, cfg.Secure),
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
		Name:     secureCookieName(CSRFCookieName, cfg.Secure),
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
		Name:     secureCookieName(SessionCookieName, cfg.Secure),
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
		Name:     secureCookieName(CSRFCookieName, cfg.Secure),
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

func getCSRFTokenFromCookie(r *http.Request, secure bool) (string, bool) {
	cookie, err := r.Cookie(secureCookieName(CSRFCookieName, secure))
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}

func getSessionTokenFromCookie(r *http.Request, secure bool) (string, bool) {
	cookie, err := r.Cookie(secureCookieName(SessionCookieName, secure))
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}

// exchangeCodeEntry holds a session token that can be claimed once via a short-lived code.
type exchangeCodeEntry struct {
	Token     string    `json:"token"`
	CSRFToken string    `json:"csrfToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// exchangeCodeStore is an in-memory store for single-use session exchange codes.
// Codes are short-lived (60s) and can be claimed exactly once.
//
// NOTE: This is a single-process, in-memory store. In multi-instance deployments,
// exchange codes issued by one instance cannot be redeemed on another. For
// horizontal scaling, this should be replaced with a Redis-backed store.
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
	s.data[code] = &exchangeCodeEntry{Token: token, CSRFToken: csrfToken, ExpiresAt: time.Now().Add(60 * time.Second)}
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
	if time.Now().After(entry.ExpiresAt) {
		delete(s.data, code)
		return "", "", false
	}
	delete(s.data, code)
	return entry.Token, entry.CSRFToken, true
}

func (s *exchangeCodeStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for code, entry := range s.data {
		if now.After(entry.ExpiresAt) {
			delete(s.data, code)
		}
	}
}

const exchangeCodeTTL = 60 * time.Second

func exchangeCodeKey(code string) string {
	sum := sha256.Sum256([]byte(code))
	return "forge:session-exchange:" + hex.EncodeToString(sum[:])
}

func issueSharedExchangeCode(ctx context.Context, cfg Config, token, csrfToken string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := hex.EncodeToString(raw)
	entry, err := json.Marshal(exchangeCodeEntry{
		Token:     token,
		CSRFToken: csrfToken,
		ExpiresAt: time.Now().Add(exchangeCodeTTL),
	})
	if err != nil {
		return "", err
	}
	if err := cfg.Redis.Set(ctx, exchangeCodeKey(code), entry, exchangeCodeTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

func claimExchangeCode(ctx context.Context, cfg Config, code string) (string, string, bool, error) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		data, err := cfg.Redis.GetDel(ctx, exchangeCodeKey(code)).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return "", "", false, nil
			}
			return "", "", false, err
		}
		var entry exchangeCodeEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return "", "", false, err
		}
		if entry.Token == "" || entry.CSRFToken == "" || time.Now().After(entry.ExpiresAt) {
			return "", "", false, nil
		}
		return entry.Token, entry.CSRFToken, true, nil
	}
	if strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production") {
		return "", "", false, errors.New("shared exchange-code storage is required in production")
	}
	token, csrfToken, ok := globalExchangeCodes.claim(code)
	return token, csrfToken, ok, nil
}

// ExchangeCodeHandler exchanges a single-use code for the session token and
// sets HttpOnly cookies directly, so the client never receives the raw JWT.
// Used by social auth callbacks to avoid placing the session token in the redirect URL.
func ExchangeCodeHandler(cfg Config) fiber.Handler {
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
		token, csrfToken, ok, err := claimExchangeCode(c.Context(), cfg, req.Code)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "exchange code storage unavailable")
		}
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "invalid or expired exchange code")
		}
		expires := tokenExpiry(cfg)
		setSessionCookies(c, token, csrfToken, expires)
		return c.JSON(fiber.Map{"ok": true})
	}
}

// issueExchangeCode creates a single-use code for the given session token.
func issueExchangeCode(ctx context.Context, cfg Config, token, csrfToken string) (string, error) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		return issueSharedExchangeCode(ctx, cfg, token, csrfToken)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production") {
		return "", errors.New("shared exchange-code storage is required in production")
	}
	return globalExchangeCodes.issue(token, csrfToken)
}
