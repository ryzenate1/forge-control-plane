package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"gamepanel/beacon/internal/logging"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

type Middleware func(http.Handler) http.Handler

func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, middleware := range middlewares {
		h = middleware(h)
	}
	return h
}

// contextKey is used for storing request-scoped values in the context.
type contextKey string

func (c contextKey) String() string { return "beacon api context key: " + string(c) }

const RequestIDKey contextKey = "request_id"

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func RecoveryMiddleware(logger logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						logging.Field{Key: "panic", Value: rec},
						logging.Field{Key: "stack", Value: string(debug.Stack())},
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(logger logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := newResponseWriter(w)
			start := time.Now()
			next.ServeHTTP(rw, r)
			logger.Info("request",
				logging.Field{Key: "method", Value: r.Method},
				logging.Field{Key: "path", Value: r.URL.Path},
				logging.Field{Key: "remote", Value: r.RemoteAddr},
				logging.Field{Key: "status", Value: rw.statusCode},
				logging.Field{Key: "duration", Value: time.Since(start).String()},
			)
		})
	}
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			w.Header().Set("X-CSRF-Token", "")
		}
		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string]*slidingWindow
}

type slidingWindow struct {
	timestamps []time.Time
}

var globalLimiter = &rateLimiter{
	requests: make(map[string]*slidingWindow),
}

const (
	maxRequestsPerMinute = 100
	windowDuration       = time.Minute
	cleanupInterval      = 5 * time.Minute
)

func init() {
	go globalLimiter.cleanupLoop()
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	sw, ok := rl.requests[ip]
	if !ok {
		sw = &slidingWindow{}
		rl.requests[ip] = sw
	}

	cutoff := now.Add(-windowDuration)
	var valid []time.Time
	for _, t := range sw.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	sw.timestamps = valid

	if len(sw.timestamps) >= maxRequestsPerMinute {
		return false
	}

	sw.timestamps = append(sw.timestamps, now)
	return true
}

func (rl *rateLimiter) cleanupLoop() {
	for {
		time.Sleep(cleanupInterval)
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-windowDuration)
		for ip, sw := range rl.requests {
			var valid []time.Time
			for _, t := range sw.timestamps {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, ip)
			} else {
				sw.timestamps = valid
			}
		}
		rl.mu.Unlock()
	}
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !globalLimiter.allow(ip) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
