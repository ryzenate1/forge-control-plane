package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gamepanel/beacon/internal/tokens"
)

func TestNewAuthMiddleware_MissingToken(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_InvalidToken(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_ExpiredToken(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	claims := tokens.Claims{
		Scope:     tokens.ScopeWebsocket,
		ServerID:  "srv-1",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	mw := NewAuthMiddleware(gen)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_ValidToken(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	claims := tokens.Claims{
		Scope:     tokens.ScopeWebsocket,
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	mw := NewAuthMiddleware(gen)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := ClaimsFromContext(r.Context())
		if got == nil {
			t.Error("expected claims in context")
		}
		if got.ServerID != "srv-1" {
			t.Errorf("expected server srv-1, got %q", got.ServerID)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers/srv-1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_WrongSecret(t *testing.T) {
	gen := tokens.NewGenerator([]byte("real-secret"))
	wrongGen := tokens.NewGenerator([]byte("wrong-secret"))
	mw := NewAuthMiddleware(wrongGen)

	claims := tokens.Claims{
		Scope:     tokens.ScopeWebsocket,
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_PublicEndpointHealth(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for public endpoint, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_PublicEndpointMetrics(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for public endpoint, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_PublicEndpointReady(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for public endpoint, got %d", w.Code)
	}
}

func TestNewAuthMiddleware_NonBearerHeader(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	mw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer header, got %d", w.Code)
	}
}

func TestRequireScopes_MatchingScope(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)
	requireMw := RequireScopes(ScopeServerRead)

	claims := tokens.Claims{
		Scope:     tokens.Scope("server:read"),
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers/srv-1", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	requireMw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 (no auth yet), got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	mw(requireMw(testHandler)).ServeHTTP(w2, req)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with matching scope, got %d", w2.Code)
	}
}

func TestRequireScopes_NoMatchingScope(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)
	requireMw := RequireScopes(ScopeAdmin)

	claims := tokens.Claims{
		Scope:     tokens.Scope("server:read"),
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	mw(requireMw(testHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for mismatched scope, got %d", w.Code)
	}
}

func TestRequireScopes_NoClaimsInContext(t *testing.T) {
	requireMw := RequireScopes(ScopeServerRead)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/servers", nil)
	w := httptest.NewRecorder()
	requireMw(testHandler).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no claims in context, got %d", w.Code)
	}
}

func TestRequireScopes_AdminScopePasses(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	claims := tokens.Claims{
		Scope:     tokens.Scope("admin"),
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()
	mw(RequireScopes(ScopeAdmin)(testHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin scope, got %d", w.Code)
	}
}

func TestRequireScopes_AnyScopeInList(t *testing.T) {
	gen := tokens.NewGenerator([]byte("test-secret"))
	mw := NewAuthMiddleware(gen)

	claims := tokens.Claims{
		Scope:     tokens.Scope("backup:read"),
		ServerID:  "srv-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	tokenStr, err := gen.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/backups", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	w := httptest.NewRecorder()
	mw(RequireScopes(ScopeServerRead, ScopeBackupRead)(testHandler)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when one of the required scopes matches, got %d", w.Code)
	}
}

func TestClaimsFromContext_NilWhenNotSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if claims := ClaimsFromContext(req.Context()); claims != nil {
		t.Error("expected nil claims when not set in context")
	}
}
