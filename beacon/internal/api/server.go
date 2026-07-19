package api

import (
	"encoding/json"
	"gamepanel/beacon/internal/auth"
	"gamepanel/beacon/internal/health"
	"gamepanel/beacon/internal/logging"
	"gamepanel/beacon/internal/metrics"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthHandler handles health check requests
func HealthHandler(checker health.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if hc, ok := checker.(*health.CompositeHealthChecker); ok {
			result := hc.CheckStatus(r.Context())
			if result.Status == "unhealthy" {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			json.NewEncoder(w).Encode(result)
			return
		}
		if err := checker.Check(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func NewServer(
	checker health.HealthChecker,
	metricsCollector metrics.MetricsCollector,
	serverHandler *ServerHandler,
	logger logging.Logger,
) *http.Server {
	r := mux.NewRouter()

	// Register server CRUD routes
	serverHandler.RegisterRoutes(r)

	// Add health check endpoint at root (for k8s probes etc.)
	r.Handle("/health", HealthHandler(checker)).Methods(http.MethodGet)

	// Add metrics endpoint at root
	r.Handle("/metrics", promhttp.Handler())

	// Log registered routes
	if err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		path, _ := route.GetPathTemplate()
		methods, _ := route.GetMethods()
		log.Printf("registered route: %s %v", path, methods)
		return nil
	}); err != nil {
		log.Printf("route walk error: %v", err)
	}

	// Apply middleware in correct order (outermost first):
	// Recovery → RequestID → Logging → CORS → CSRF → Auth → RateLimit → ErrorHandler → ValidateRequest
	handler := Chain(
		r,
		RecoveryMiddleware(logger),
		RequestIDMiddleware,
		LoggingMiddleware(logger),
		CORSMiddleware,
		CSRFMiddleware,
		auth.AuthMiddleware,
		RateLimitMiddleware,
		ErrorHandler,
		ValidateRequest,
	)

	// Set up versioned routes under /v1/
	versionedHandlers := []VersionedHandler{
		{
			Version: CurrentVersion,
			Handler: handler,
		},
	}
	versionedRouter := VersionedRouter(versionedHandlers...)

	// Root-level mux ensures /health is accessible without /v1/ prefix
	rootMux := http.NewServeMux()
	rootMux.Handle("/health", HealthHandler(checker))
	rootMux.Handle("/", versionedRouter)

	return &http.Server{
		Handler:           rootMux,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
