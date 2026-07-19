package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion == "" {
		t.Error("expected CurrentVersion to be non-empty")
	}
}

func TestVersionedRouter(t *testing.T) {
	var handled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled = true
		w.WriteHeader(http.StatusOK)
	})

	mux := VersionedRouter(VersionedHandler{
		Version: CurrentVersion,
		Handler: handler,
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/" + CurrentVersion + "/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !handled {
		t.Error("expected handler to be called")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestVersionedRouter_StripsPrefix(t *testing.T) {
	var capturedPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	mux := VersionedRouter(VersionedHandler{
		Version: "v2",
		Handler: handler,
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v2/resource/123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if !strings.HasPrefix(capturedPath, "/resource/123") {
		t.Errorf("expected path starting with /resource/123, got %q", capturedPath)
	}
}

func TestVersionedRouter_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := VersionedRouter(VersionedHandler{
		Version: "v1",
		Handler: handler,
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v3/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown version, got %d", resp.StatusCode)
	}
}

func TestVersionedRouter_MultipleVersions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := VersionedRouter(
		VersionedHandler{Version: "v1", Handler: handler},
		VersionedHandler{Version: "v2", Handler: handler},
	)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v2/foo")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
