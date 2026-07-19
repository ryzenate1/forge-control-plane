package api

import (
	"gamepanel/beacon/internal/logging"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareChain(t *testing.T) {
	var order []string
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1")
			next.ServeHTTP(w, r)
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2")
			next.ServeHTTP(w, r)
		})
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "final")
		w.WriteHeader(http.StatusOK)
	})

	h := Chain(final, m1, m2)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	expected := []string{"m2", "m1", "final"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, s := range expected {
		if order[i] != s {
			t.Errorf("step %d: expected %s, got %s", i, s, order[i])
		}
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	h := RecoveryMiddleware(logging.NewNoopLogger())(panicking)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "Internal Server Error\n" && body != "Internal Server Error" {
		t.Errorf("expected 'Internal Server Error', got %q", body)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	var capturedID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID, _ = r.Context().Value(RequestIDKey).(string)
		w.WriteHeader(http.StatusOK)
	})
	h := RequestIDMiddleware(next)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	header := w.Header().Get("X-Request-ID")
	if header == "" {
		t.Error("expected non-empty X-Request-ID header")
	}
	if header != capturedID {
		t.Errorf("context request ID %q does not match header %q", capturedID, header)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := LoggingMiddleware(logging.NewNoopLogger())(next)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("sets CORS headers", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := CORSMiddleware(next)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(w, r)

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Error("expected Access-Control-Allow-Origin: *")
		}
		if w.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Error("expected non-empty Access-Control-Allow-Methods")
		}
		if w.Header().Get("Access-Control-Allow-Headers") == "" {
			t.Error("expected non-empty Access-Control-Allow-Headers")
		}
		if w.Header().Get("Access-Control-Max-Age") == "" {
			t.Error("expected non-empty Access-Control-Max-Age")
		}
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("OPTIONS returns 200 without calling next", func(t *testing.T) {
		var called bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})
		h := CORSMiddleware(next)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "/", nil)
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if called {
			t.Error("expected next handler NOT to be called for OPTIONS")
		}
	})
}

func TestCSRFMiddleware(t *testing.T) {
	t.Run("sets X-CSRF-Token for safe methods", func(t *testing.T) {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			h := CSRFMiddleware(next)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, "/", nil)
			h.ServeHTTP(w, r)

			if w.Header().Get("X-CSRF-Token") != "" {
				t.Errorf("expected X-CSRF-Token to be set for %s", method)
			}
			if !called {
				t.Errorf("expected next handler to be called for %s", method)
			}
		}
	})

	t.Run("does not set X-CSRF-Token for unsafe methods", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := CSRFMiddleware(next)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		h.ServeHTTP(w, r)

		if w.Header().Get("X-CSRF-Token") != "" {
			t.Error("expected no X-CSRF-Token for POST")
		}
	})
}

func TestRateLimitMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RateLimitMiddleware(next)

	// Rate limiter uses RemoteAddr; vary it so current test runs don't affect this one.
	passed := 0
	blocked := 0
	for i := 0; i < maxRequestsPerMinute+10; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "192.0.2.1:12345"
		h.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			passed++
		} else if w.Code == http.StatusTooManyRequests {
			blocked++
		}
	}
	if passed != maxRequestsPerMinute {
		t.Errorf("expected %d passed requests, got %d", maxRequestsPerMinute, passed)
	}
	if blocked == 0 {
		t.Error("expected at least one blocked request")
	}
}

func TestErrorHandler(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})
	h := ErrorHandler(next)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	if !called {
		t.Error("expected next handler to be called")
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", w.Code)
	}
}
