package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gamepanel/beacon/internal/runtime"
	daemonhttp "gamepanel/beacon/internal/server"
)

type testPinger struct {
	called bool
	err    error
}

type recoveryRuntime struct {
	starts   atomic.Int32
	restarts atomic.Int32
}

type recoveryConsole struct{}

func (*recoveryConsole) Read([]byte) (int, error)    { return 0, io.EOF }
func (*recoveryConsole) Write(p []byte) (int, error) { return len(p), nil }
func (*recoveryConsole) Close() error                { return nil }

func (*recoveryRuntime) Create(context.Context, runtime.CreateRequest) error { return nil }
func (*recoveryRuntime) Install(context.Context, runtime.InstallRequest) (runtime.InstallResult, error) {
	return runtime.InstallResult{}, nil
}
func (*recoveryRuntime) Inspect(context.Context, string) (runtime.ContainerState, error) {
	return runtime.ContainerState{Exists: true, Running: true}, nil
}
func (*recoveryRuntime) List(context.Context) ([]runtime.ContainerState, error) { return nil, nil }
func (r *recoveryRuntime) Start(context.Context, string) error {
	r.starts.Add(1)
	return nil
}
func (*recoveryRuntime) SendCommand(context.Context, string, string) error { return nil }
func (*recoveryRuntime) Stop(context.Context, string) error                { return nil }
func (*recoveryRuntime) WaitForStop(context.Context, string, time.Duration, bool) error {
	return nil
}
func (*recoveryRuntime) Kill(context.Context, string) error           { return nil }
func (*recoveryRuntime) Signal(context.Context, string, string) error { return nil }
func (r *recoveryRuntime) Restart(context.Context, string) error {
	r.restarts.Add(1)
	return nil
}
func (*recoveryRuntime) Stats(context.Context, string) (runtime.Stats, error) {
	return runtime.Stats{}, nil
}
func (*recoveryRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (*recoveryRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (*recoveryRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (*recoveryRuntime) AttachConsole(context.Context, string) (runtime.ConsoleSession, error) {
	return &recoveryConsole{}, nil
}
func (*recoveryRuntime) Delete(context.Context, string) error { return nil }

func (p *testPinger) Ping(context.Context) error {
	p.called = true
	return p.err
}

func TestValidatePanelOnboarding(t *testing.T) {
	tests := []struct {
		name                    string
		nodeID, panelURL, token string
		wantErr                 string
	}{
		{name: "standalone daemon authentication", token: "node-token"},
		{name: "complete onboarding", nodeID: "node-1", panelURL: "https://panel.example.com/api", token: "node-token"},
		{name: "node ID without panel URL", nodeID: "node-1", token: "node-token", wantErr: "DAEMON_NODE_ID and PANEL_API_URL together"},
		{name: "panel URL without node ID", panelURL: "https://panel.example.com", token: "node-token", wantErr: "DAEMON_NODE_ID and PANEL_API_URL together"},
		{name: "onboarding without token", nodeID: "node-1", panelURL: "https://panel.example.com", wantErr: "DAEMON_NODE_TOKEN"},
		{name: "invalid panel URL", nodeID: "node-1", panelURL: "ftp://panel.example.com", token: "node-token", wantErr: "absolute http(s) URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePanelOnboarding(tt.nodeID, tt.panelURL, tt.token)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validatePanelOnboarding() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validatePanelOnboarding() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDockerHeartbeatStatusPingsRuntime(t *testing.T) {
	pinger := &testPinger{}
	status, detail := runtimeHeartbeatStatus(pinger, "docker")
	if !pinger.called || status != "ok" || detail != "" {
		t.Fatalf("unexpected successful status: called=%v status=%q detail=%q", pinger.called, status, detail)
	}

	pinger = &testPinger{err: errors.New("daemon unavailable")}
	status, detail = runtimeHeartbeatStatus(pinger, "docker")
	if !pinger.called || status != "error" || !strings.Contains(detail, "daemon unavailable") {
		t.Fatalf("unexpected failed status: called=%v status=%q detail=%q", pinger.called, status, detail)
	}
}

func TestPanelServerStateExtractsReconstructionFlags(t *testing.T) {
	disk, suspended, installation := panelServerState([]byte(`{
		"suspended": true,
		"is_installing": true,
		"build": {"disk_space": 4096}
	}`))
	if disk != 4096 || !suspended || installation != "installing" {
		t.Fatalf("unexpected panel state: disk=%d suspended=%v installation=%q", disk, suspended, installation)
	}

	disk, suspended, installation = panelServerState([]byte(`{"installed":false,"disk_mb":512}`))
	if disk != 512 || suspended || installation != "uninstalled" {
		t.Fatalf("unexpected uninstalled panel state: disk=%d suspended=%v installation=%q", disk, suspended, installation)
	}
}

func TestRecoverServersFromDiskRestoresPowerOperations(t *testing.T) {
	const serverID = "123e4567-e89b-12d3-a456-426614174099"
	dataDir := t.TempDir()
	configDir := filepath.Join(dataDir, serverID, ".config")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.json"), []byte(`{"settings":{"build":{"disk_space":64}},"installed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rt := &recoveryRuntime{}
	server, handler := daemonhttp.NewServer(rt, dataDir)
	defer server.Shutdown()

	if err := recoverServersFromDisk(context.Background(), dataDir, server); err != nil {
		t.Fatalf("recover servers: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/servers/"+serverID+"/power", strings.NewReader(`{"signal":"restart"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected recovered server power operation to succeed, got %d: %s", rec.Code, rec.Body.String())
	}
	if rt.starts.Load() != 1 {
		t.Fatalf("expected recovered restart operation to start the container once, got %d", rt.starts.Load())
	}
}

func TestDockerHeartbeatStatusRejectsMissingRuntime(t *testing.T) {
	status, detail := runtimeHeartbeatStatus(nil, "docker")
	if status != "error" || detail != "docker runtime unavailable" {
		t.Fatalf("unexpected missing-runtime status: %q %q", status, detail)
	}
}

func TestBuildBackupAdapterFailsClosedForIncompleteS3Configuration(t *testing.T) {
	t.Setenv("BACKUP_ADAPTER", "s3")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_ACCESS_KEY_ID", "")
	t.Setenv("S3_SECRET_ACCESS_KEY", "")
	adapter, err := buildBackupAdapter(t.TempDir())
	if err == nil || adapter != nil {
		t.Fatalf("incomplete S3 configuration must fail closed: adapter=%T err=%v", adapter, err)
	}
}

func TestBuildBackupAdapterRejectsUnknownAdapter(t *testing.T) {
	t.Setenv("BACKUP_ADAPTER", "unknown")
	adapter, err := buildBackupAdapter(t.TempDir())
	if err == nil || adapter != nil {
		t.Fatalf("unknown adapter must fail closed: adapter=%T err=%v", adapter, err)
	}
}
