package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testServerID = "123e4567-e89b-12d3-a456-426614174000"

func newTestHandler(t *testing.T, token ...string) http.Handler {
	t.Helper()
	_, handler := NewServer(nil, t.TempDir(), token...)
	return handler
}

func newTestHandlerWithDir(t *testing.T, dataDir string, token ...string) http.Handler {
	t.Helper()
	_, handler := NewServer(nil, dataDir, token...)
	return handler
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestMetricsArePublicWhenTokenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "game_panel_daemon_uptime_seconds") {
		t.Fatalf("expected daemon metrics, got %q", rec.Body.String())
	}
}

func TestPowerRejectsInvalidSignal(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/power", strings.NewReader(`{"signal":"explode"}`))
	rec := httptest.NewRecorder()

	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestCreateRejectsUnavailableRuntime(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(`{"serverId":"`+testServerID+`","image":"busybox","memoryMb":128,"cpuShares":128}`))
	rec := httptest.NewRecorder()

	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestSignedRequestsAreRequiredWhenTokenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(`{"serverId":"`+testServerID+`","image":"busybox"}`))
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestSignedRequestReachesUnavailableRuntime(t *testing.T) {
	body := []byte(`{"serverId":"` + testServerID + `","image":"busybox"}`)
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewReader(body))
	timestamp := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Signature", sign("secret", req.Method, req.URL.RequestURI(), timestamp, body))
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestFileAPIRejectsPathTraversal(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files?path=../outside", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestFileAPIRejectsSymlinkEscape(t *testing.T) {
	dataDir := t.TempDir()
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(serverDir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files?path=escape", nil)
	rec := httptest.NewRecorder()

	newTestHandlerWithDir(t, dataDir).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestChunkedUploadAssemblesFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)

	first := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=config/server.properties&uploadId=test-upload&offset=0&final=false", strings.NewReader("hello "))
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first chunk status 200, got %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=config/server.properties&uploadId=test-upload&offset=6&final=true", strings.NewReader("world"))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected final chunk status 200, got %d", secondRec.Code)
	}

	body, err := os.ReadFile(filepath.Join(dataDir, testServerID, "config", "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello world" {
		t.Fatalf("unexpected file body %q", body)
	}
}

func TestChunkedUploadRejectsOffsetMismatch(t *testing.T) {
	handler := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=server.properties&uploadId=test-upload&offset=12&final=false", strings.NewReader("hello"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}
