package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gamepanel/beacon/internal/runtime"
)

type stubRuntime struct {
	createErr    error
	installErr   error
	startErr     error
	stopErr      error
	killErr      error
	statsErr     error
	logsErr      error
	consoleErr   error
	deleteErr    error
	createCalled bool
	deleteCalled bool
	createReq    runtime.CreateRequest
}

func (r *stubRuntime) Create(_ context.Context, req runtime.CreateRequest) error {
	r.createCalled = true
	r.createReq = req
	return r.createErr
}

func (r *stubRuntime) Install(context.Context, runtime.InstallRequest) (runtime.InstallResult, error) {
	return runtime.InstallResult{}, r.installErr
}

func (r *stubRuntime) Inspect(context.Context, string) (runtime.ContainerState, error) {
	return runtime.ContainerState{Exists: true}, nil
}
func (r *stubRuntime) List(context.Context) ([]runtime.ContainerState, error) { return nil, nil }
func (r *stubRuntime) Start(context.Context, string) error                    { return r.startErr }
func (r *stubRuntime) SendCommand(context.Context, string, string) error {
	return nil
}
func (r *stubRuntime) Stop(context.Context, string) error { return r.stopErr }
func (r *stubRuntime) WaitForStop(context.Context, string, time.Duration, bool) error {
	return r.stopErr
}
func (r *stubRuntime) Kill(context.Context, string) error           { return r.killErr }
func (r *stubRuntime) Signal(context.Context, string, string) error { return nil }
func (r *stubRuntime) Restart(context.Context, string) error        { return r.startErr }
func (r *stubRuntime) Stats(context.Context, string) (runtime.Stats, error) {
	return runtime.Stats{}, r.statsErr
}
func (r *stubRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	if r.logsErr != nil {
		return nil, r.logsErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *stubRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	if r.logsErr != nil {
		return nil, r.logsErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *stubRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	if r.statsErr != nil {
		return nil, r.statsErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *stubRuntime) AttachConsole(context.Context, string) (runtime.ConsoleSession, error) {
	if r.consoleErr != nil {
		return nil, r.consoleErr
	}
	return &stubConsole{}, nil
}
func (r *stubRuntime) Delete(context.Context, string) error {
	r.deleteCalled = true
	return r.deleteErr
}

type stubConsole struct {
	bytes.Buffer
}

func (*stubConsole) Close() error { return nil }

func TestRuntimeBackedEndpointsReturn503WithoutRuntime(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create", http.MethodPost, "/servers", `{"serverId":"` + testServerID + `","image":"busybox"}`},
		{"install", http.MethodPost, "/servers/" + testServerID + "/install", `{}`},
		{"install websocket", http.MethodGet, "/servers/" + testServerID + "/install/ws", ""},
		{"delete", http.MethodDelete, "/servers/" + testServerID, ""},
		{"power", http.MethodPost, "/servers/" + testServerID + "/power", `{"signal":"kill"}`},
		{"stats", http.MethodGet, "/servers/" + testServerID + "/stats", ""},
		{"stats websocket", http.MethodGet, "/servers/" + testServerID + "/ws/stats", ""},
		{"logs", http.MethodGet, "/servers/" + testServerID + "/logs", ""},
		{"logs websocket", http.MethodGet, "/servers/" + testServerID + "/ws/logs", ""},
		{"command", http.MethodPost, "/servers/" + testServerID + "/command", `{"command":"status"}`},
		{"console websocket", http.MethodGet, "/servers/" + testServerID + "/ws/console", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			rec := httptest.NewRecorder()
			newTestHandler(t).ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected status 503, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeleteRemovesRuntimeAndServerData(t *testing.T) {
	dataDir := t.TempDir()
	serverRoot := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(filepath.Join(serverRoot, ".config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "world.dat"), []byte("tenant data"), 0o640); err != nil {
		t.Fatal(err)
	}
	rt := &stubRuntime{}
	_, handler := NewServer(rt, dataDir)
	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !rt.deleteCalled {
		t.Fatal("expected runtime container deletion")
	}
	if _, err := os.Stat(serverRoot); !os.IsNotExist(err) {
		t.Fatalf("expected server data root to be removed, stat error = %v", err)
	}
}

func TestCreatePassesCanonicalRootToRuntime(t *testing.T) {
	dataDir := t.TempDir()
	rt := &stubRuntime{}
	_, handler := NewServer(rt, dataDir)
	req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(`{"serverId":"`+testServerID+`","image":"busybox"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", rec.Code, rec.Body.String())
	}
	wantRoot := filepath.Join(dataDir, testServerID)
	if !rt.createCalled || rt.createReq.RootDir != wantRoot || !filepath.IsAbs(rt.createReq.RootDir) {
		t.Fatalf("unexpected create request root: called=%v root=%q want=%q", rt.createCalled, rt.createReq.RootDir, wantRoot)
	}
	info, err := os.Stat(rt.createReq.RootDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected create root directory to exist: info=%v err=%v", info, err)
	}
	if strings.Contains(rec.Body.String(), "mock") {
		t.Fatalf("unexpected mock response: %s", rec.Body.String())
	}
}

func TestCreateRejectsTraversalServerID(t *testing.T) {
	dataDir := t.TempDir()
	rt := &stubRuntime{}
	_, handler := NewServer(rt, dataDir)
	req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(`{"serverId":"../outside","image":"busybox"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if rt.createCalled {
		t.Fatal("runtime create was called for a traversal id")
	}
}

func TestIncomingTransferRejectsTraversalServerID(t *testing.T) {
	dataDir := t.TempDir()
	_, handler := NewServer(nil, dataDir)
	req := httptest.NewRequest(http.MethodPost, "/api/transfers", strings.NewReader("archive"))
	req.Header.Set("X-Transfer-ServerID", "../../outside")
	req.Header.Set("X-Transfer-ID", "transfer-id")
	req.Header.Set("X-Checksum", "checksum")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "outside")); !os.IsNotExist(err) {
		t.Fatalf("traversal request created an outside path: %v", err)
	}
}

func TestIncomingTransferResumesPersistedArchive(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, testServerID), 0o750); err != nil {
		t.Fatal(err)
	}
	_, handler := NewServer(nil, dataDir)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	payload := []byte("resumed")
	if err := tw.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0o600, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	body := archive.Bytes()
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	split := len(body) / 2

	send := func(offset int, chunk []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/transfers", bytes.NewReader(chunk))
		req.Header.Set("X-Transfer-ServerID", testServerID)
		req.Header.Set("X-Transfer-ID", "transfer-id")
		req.Header.Set("X-Transfer-Resume-Offset", strconv.Itoa(offset))
		req.Header.Set("X-Transfer-Size", strconv.Itoa(len(body)))
		req.Header.Set("X-Checksum", checksum)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := send(0, body[:split]); rec.Code != http.StatusAccepted {
		t.Fatalf("initial partial upload returned %d: %s", rec.Code, rec.Body.String())
	}
	if rec := send(split, body[split:]); rec.Code != http.StatusOK {
		t.Fatalf("resumed upload returned %d: %s", rec.Code, rec.Body.String())
	}
	extracted, err := os.ReadFile(filepath.Join(dataDir, testServerID, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(extracted, payload) {
		t.Fatalf("unexpected extracted payload: %q", extracted)
	}
}

func TestMissingRuntimeResourcesReturn404(t *testing.T) {
	missingContainer := errors.New("No such container: mgp-server")
	missingImage := errors.New("pull access denied for missing-image")
	tests := []struct {
		name   string
		rt     *stubRuntime
		method string
		path   string
		body   string
	}{
		{"create image", &stubRuntime{createErr: missingImage}, http.MethodPost, "/servers", `{"serverId":"` + testServerID + `","image":"missing"}`},
		{"install image", &stubRuntime{installErr: missingImage}, http.MethodPost, "/servers/" + testServerID + "/install", `{"image":"missing"}`},
		{"delete container", &stubRuntime{deleteErr: missingContainer}, http.MethodDelete, "/servers/" + testServerID, ""},
		{"power container", &stubRuntime{killErr: missingContainer}, http.MethodPost, "/servers/" + testServerID + "/power", `{"signal":"kill"}`},
		{"stats container", &stubRuntime{statsErr: missingContainer}, http.MethodGet, "/servers/" + testServerID + "/stats", ""},
		{"logs container", &stubRuntime{logsErr: missingContainer}, http.MethodGet, "/servers/" + testServerID + "/logs", ""},
		{"console container", &stubRuntime{consoleErr: missingContainer}, http.MethodPost, "/servers/" + testServerID + "/command", `{"command":"status"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, handler := NewServer(test.rt, t.TempDir())
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
