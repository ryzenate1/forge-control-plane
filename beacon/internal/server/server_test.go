package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gamepanel/beacon/internal/backup"
	"gamepanel/beacon/internal/tokens"
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

func newTestHandlerWithBackup(t *testing.T, backups *testBackupAdapter) http.Handler {
	t.Helper()
	_, handler := NewServerWithBackup(nil, t.TempDir(), backups)
	return handler
}

// testBackupAdapter is an in-memory backup.BackupInterface for handler tests.
type testBackupAdapter struct {
	mu               sync.Mutex
	entries          map[string]map[string][]byte
	lastRestorePaths []string
	lastTruncate     bool
}

func newTestBackupAdapter() *testBackupAdapter {
	return &testBackupAdapter{entries: make(map[string]map[string][]byte)}
}

func (a *testBackupAdapter) Create(ctx context.Context, serverRoot, backupDir, name string, ignored []string) (*backup.BackupInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.entries[backupDir] == nil {
		a.entries[backupDir] = make(map[string][]byte)
	}
	a.entries[backupDir][name] = []byte("archive:" + name)
	return &backup.BackupInfo{UUID: strings.TrimSuffix(name, ".zip"), Name: name, Status: "completed", Adapter: "test"}, nil
}

func (a *testBackupAdapter) List(backupDir string) ([]backup.BackupInfo, error) { return nil, nil }

func (a *testBackupAdapter) Get(backupDir, name string) (*backup.BackupInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.entries[backupDir][name]; ok {
		return &backup.BackupInfo{UUID: strings.TrimSuffix(name, ".zip"), Name: name, Status: "completed", Adapter: "test"}, nil
	}
	return nil, os.ErrNotExist
}

func (a *testBackupAdapter) Delete(backupDir, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.entries[backupDir][name]; !ok {
		return os.ErrNotExist
	}
	delete(a.entries[backupDir], name)
	return nil
}

func (a *testBackupAdapter) Restore(ctx context.Context, backupDir, name, serverRoot string, truncate bool, paths []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastRestorePaths = append([]string(nil), paths...)
	a.lastTruncate = truncate
	return nil
}

func (a *testBackupAdapter) Download(backupDir, name string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, ok := a.entries[backupDir][name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (a *testBackupAdapter) SetProgressCallback(fn backup.ProgressFunc) {}

func (a *testBackupAdapter) Type() backup.AdapterType { return "test" }

func (a *testBackupAdapter) lastRestore() ([]string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.lastRestorePaths...), a.lastTruncate
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestReadyIsPublicProbe(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	// The readiness probe must be reachable without authentication. With a nil
	// test runtime the endpoint legitimately reports not-ready (503); the
	// important contract is that it is not rejected as unauthorized (401).
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("unauthenticated readiness probe must not be rejected: %s", rec.Body.String())
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected runtime-unavailable 503 for nil runtime, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMetricsRequireAuthenticationWhenTokenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "game_panel_daemon_uptime_seconds") {
		t.Fatalf("unauthenticated response leaked daemon metrics: %q", rec.Body.String())
	}
}

func TestMetricsExposeProcessAndContainerMetrics(t *testing.T) {
	server, handler := NewServer(nil, t.TempDir(), "secret")
	server.SetMetricsToken("secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	for _, expected := range []string{
		"game_panel_daemon_uptime_seconds",
		"game_panel_daemon_runtime_enabled",
		"game_panel_daemon_cpu_user_seconds_total",
		"game_panel_daemon_cpu_system_seconds_total",
		"game_panel_daemon_goroutines",
		"game_panel_daemon_memory_alloc_bytes",
		"game_panel_daemon_memory_heap_bytes",
		"game_panel_daemon_gc_total",
	} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Fatalf("metrics output missing %q:\n%s", expected, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "game_panel_daemon_container_cpu_percent{server_id=") {
		t.Fatalf("container metrics emitted without a runtime")
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

func TestBackupDeleteAcceptsBareIdentifier(t *testing.T) {
	adapter := newTestBackupAdapter()
	const stored = "backup-20260101T000000Z.zip"
	if _, err := adapter.Create(context.Background(), "", testServerID, stored, nil); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandlerWithBackup(t, adapter)

	// The client sends the bare identifier without the ".zip" suffix.
	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/backups/backup-20260101T000000Z", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := adapter.Get(testServerID, stored); err == nil {
		t.Fatalf("backup %q was not deleted from the adapter", stored)
	}
}

func TestBackupRestoreAcceptsBareIdentifier(t *testing.T) {
	adapter := newTestBackupAdapter()
	const stored = "backup-20260101T000000Z.zip"
	if _, err := adapter.Create(context.Background(), "", testServerID, stored, nil); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandlerWithBackup(t, adapter)

	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/backups/restore",
		strings.NewReader(`{"name":"backup-20260101T000000Z","truncate":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), stored) {
		t.Fatalf("response did not reference the canonical stored name %q: %s", stored, rec.Body.String())
	}
}

func TestBackupRestoreForwardsRequestedPaths(t *testing.T) {
	adapter := newTestBackupAdapter()
	const stored = "backup-20260101T000000Z.zip"
	if _, err := adapter.Create(context.Background(), "", testServerID, stored, nil); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandlerWithBackup(t, adapter)

	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/backups/restore",
		strings.NewReader(`{"name":"backup-20260101T000000Z.zip","paths":["plugins/foo.jar","world"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	paths, _ := adapter.lastRestore()
	want := []string{"plugins/foo.jar", "world"}
	if len(paths) != len(want) || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("restore paths = %v, want %v", paths, want)
	}
}

func TestBackupRestoreWithoutPathsIsFullRestore(t *testing.T) {
	adapter := newTestBackupAdapter()
	const stored = "backup-20260101T000000Z.zip"
	if _, err := adapter.Create(context.Background(), "", testServerID, stored, nil); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandlerWithBackup(t, adapter)

	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/backups/restore",
		strings.NewReader(`{"name":"backup-20260101T000000Z.zip","truncate":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	paths, truncate := adapter.lastRestore()
	if len(paths) != 0 {
		t.Fatalf("full restore forwarded paths %v, want none", paths)
	}
	if !truncate {
		t.Fatalf("full restore did not forward truncate=true")
	}
}

func TestBackupHandlersRejectMaliciousNames(t *testing.T) {
	adapter := newTestBackupAdapter()
	handler := newTestHandlerWithBackup(t, adapter)

	malicious := []string{"../escape", "../../etc/passwd", "/etc/passwd", "backup/../../x", `backup\evil`, "backup..zip"}
	for _, name := range malicious {
		t.Run("restore "+name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/backups/restore",
				strings.NewReader(`{"name":"`+name+`"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("restore with %q = %d, want 400", name, rec.Code)
			}
		})
		t.Run("delete "+name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/backups?name="+url.QueryEscape(name), nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("delete with %q = %d, want 400", name, rec.Code)
			}
		})
	}
}

func TestNormalizeBackupName(t *testing.T) {
	valid := map[string]string{
		"backup-20260101T000000Z":              "backup-20260101T000000Z.zip",
		"backup-20260101T000000Z.zip":          "backup-20260101T000000Z.zip",
		"123e4567-e89b-12d3-a456-426614174000": "123e4567-e89b-12d3-a456-426614174000.zip",
	}
	for input, want := range valid {
		got, ok := normalizeBackupName(input)
		if !ok || got != want {
			t.Fatalf("normalizeBackupName(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"", "..", "../escape", "dir/name", `dir\name`, "/absolute", "backup..zip", strings.Repeat("a", 129)} {
		if got, ok := normalizeBackupName(input); ok {
			t.Fatalf("normalizeBackupName(%q) = %q, accepted malicious input", input, got)
		}
	}
}

func TestSignedRequestReachesUnavailableRuntime(t *testing.T) {
	body := []byte(`{"serverId":"` + testServerID + `","image":"busybox"}`)
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewReader(body))
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "0123456789abcdef0123456789abcdef"
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", sign("secret", req.Method, req.URL.RequestURI(), timestamp, body, nonce))
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestSignedRequestNonceCannotBeReplayed(t *testing.T) {
	body := []byte(`{"serverId":"` + testServerID + `","image":"busybox"}`)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "abcdef0123456789abcdef0123456789"
	handler := newTestHandler(t, "secret")
	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewReader(body))
		req.Header.Set("X-Panel-Timestamp", timestamp)
		req.Header.Set("X-Panel-Nonce", nonce)
		req.Header.Set("X-Panel-Signature", sign("secret", req.Method, req.URL.RequestURI(), timestamp, body, nonce))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if status := send(); status == http.StatusUnauthorized {
		t.Fatalf("first signed request was rejected as replay")
	}
	if status := send(); status != http.StatusUnauthorized {
		t.Fatalf("replayed signed request status = %d, want 401", status)
	}
}

type readTrackingBody struct{ read bool }

func (r *readTrackingBody) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func (r *readTrackingBody) Close() error { return nil }

func TestStreamingUploadRejectsInvalidSignatureBeforeReadingBody(t *testing.T) {
	body := &readTrackingBody{}
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=file.bin", body)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", "22222222222222222222222222222222")
	req.Header.Set("X-Panel-Signature", "invalid")
	rec := httptest.NewRecorder()

	newTestHandler(t, "secret").ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if body.read {
		t.Fatal("unauthenticated streaming body was read")
	}
}

func TestTransferDestinationRequiresHTTPSOutsideLoopback(t *testing.T) {
	for _, raw := range []string{"http://node.internal", "ftp://node.example", "https://user:pass@node.example"} {
		if err := validateTransferDestination(raw); err == nil {
			t.Errorf("validateTransferDestination(%q) unexpectedly succeeded", raw)
		}
	}
	for _, raw := range []string{"https://node.example", "http://127.0.0.1:9090"} {
		if err := validateTransferDestination(raw); err != nil {
			t.Errorf("validateTransferDestination(%q): %v", raw, err)
		}
	}
}

func TestWebSocketOriginValidation(t *testing.T) {
	previous := allowedWebSocketOrigins
	allowedWebSocketOrigins = []string{"https://panel.example"}
	t.Cleanup(func() { allowedWebSocketOrigins = previous })
	for origin, want := range map[string]bool{
		"":                      true,
		"https://panel.example": true,
		"https://evil.example":  false,
		"://invalid":            false,
	} {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if got := websocketUpgrader.CheckOrigin(req); got != want {
			t.Errorf("origin %q accepted=%t, want %t", origin, got, want)
		}
	}
}

func TestContainerExecRejectsInterpreterBeforeDockerAccess(t *testing.T) {
	body := []byte(`{"cmd":["python","-c","print(1)"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/containers/example/exec", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "11111111111111111111111111111111"
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", sign("secret", req.Method, req.URL.RequestURI(), timestamp, body, nonce))
	rec := httptest.NewRecorder()
	newTestHandler(t, "secret").ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("interpreter exec status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestScopedWebSocketTokenReachesRouteWithoutPanelSignature(t *testing.T) {
	server, handler := NewServer(&stubRuntime{}, t.TempDir(), "secret")
	generator := tokens.NewGenerator([]byte("secret"))
	server.SetTokenGenerator(generator)
	token, err := generator.GenerateWebsocket(testServerID, "user-1", time.Minute)
	if err != nil {
		t.Fatalf("generate websocket token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/ws/stats?token="+token, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected websocket upgrade to reach route and fail with 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "signature") {
		t.Fatalf("scoped websocket request was incorrectly handled as panel HMAC: %s", rec.Body.String())
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

func TestUploadRejectsOversizedContentLength(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=big.bin&uploadId=test-upload&offset=0&final=false", strings.NewReader(""))
	req.ContentLength = maxUploadChunkBytes + 1
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, testServerID, ".uploads")); !os.IsNotExist(err) {
		t.Fatal("oversized request must not create a spool directory")
	}
}

func TestUploadAbortsOnInsufficientDiskSpace(t *testing.T) {
	dataDir := t.TempDir()
	server, handler := NewServer(nil, dataDir)
	server.diskFreeFn = func(string) (int64, error) { return 0, nil }
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=blocked.bin&uploadId=test-upload&offset=0&final=false", strings.NewReader("hello"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected status 507, got %d", rec.Code)
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, testServerID, ".uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("aborted upload left spool files: %+v", entries)
	}
}

func TestWriteFileAbortsOnInsufficientDiskSpace(t *testing.T) {
	dataDir := t.TempDir()
	server, handler := NewServer(nil, dataDir)
	server.diskFreeFn = func(string) (int64, error) { return 0, nil }
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/content?path=blocked.txt", strings.NewReader("hello"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected status 507, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, testServerID, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatal("aborted write must not leave a destination file")
	}
}
