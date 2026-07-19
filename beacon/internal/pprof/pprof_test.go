package pprof

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRegisterRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux)

	paths := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
		"/debug/pprof/allocs",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code == http.StatusNotFound {
			t.Errorf("route %s not registered", path)
		}
	}
}

func TestIsEnabled_Default(t *testing.T) {
	os.Unsetenv("DAEMON_PPROF_ENABLED")
	if IsEnabled() {
		t.Error("expected disabled by default")
	}
}

func TestIsEnabled_True(t *testing.T) {
	os.Setenv("DAEMON_PPROF_ENABLED", "true")
	defer os.Unsetenv("DAEMON_PPROF_ENABLED")
	if !IsEnabled() {
		t.Error("expected enabled when set to true")
	}
}

func TestIsEnabled_False(t *testing.T) {
	os.Setenv("DAEMON_PPROF_ENABLED", "false")
	defer os.Unsetenv("DAEMON_PPROF_ENABLED")
	if IsEnabled() {
		t.Error("expected disabled when set to false")
	}
}
