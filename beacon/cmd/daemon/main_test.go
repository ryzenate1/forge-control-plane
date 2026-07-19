package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type testPinger struct {
	called bool
	err    error
}

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
