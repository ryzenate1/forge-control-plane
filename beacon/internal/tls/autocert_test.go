package tls

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAutoTLSManager(t *testing.T) {
	dir := t.TempDir()
	mgr := NewAutoTLSManager("example.com", dir, "admin@example.com")
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.hostname != "example.com" {
		t.Errorf("expected hostname example.com, got %s", mgr.hostname)
	}
	if mgr.cacheDir != dir {
		t.Errorf("expected cacheDir %s, got %s", dir, mgr.cacheDir)
	}
}

func TestNewAutoTLSManager_NoEmail(t *testing.T) {
	dir := t.TempDir()
	mgr := NewAutoTLSManager("example.com", dir, "")
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestAutoTLSManager_HTTPHandler(t *testing.T) {
	dir := t.TempDir()
	mgr := NewAutoTLSManager("example.com", dir, "")
	handler := mgr.HTTPHandler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Log("handler responded (ACME challenge endpoint)")
	}
}

func TestAutoTLSManager_GetCertificate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewAutoTLSManager("example.com", dir, "")
	if mgr.manager == nil {
		t.Fatal("expected non-nil underlying autocert manager")
	}
}
