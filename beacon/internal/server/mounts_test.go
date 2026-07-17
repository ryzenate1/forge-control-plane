package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gamepanel/beacon/internal/runtime"
)

func TestCreateAllowsOnlyConfiguredMountSources(t *testing.T) {
	allowed := t.TempDir()
	allowedChild := filepath.Join(allowed, "child")
	if err := osMkdirAll(allowedChild); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	tests := []struct {
		name   string
		source string
		status int
	}{
		{name: "allowed descendant", source: allowedChild, status: http.StatusAccepted},
		{name: "outside configured root", source: outside, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := &mountTestRuntime{}
			server, handler := NewServer(rt, t.TempDir())
			server.SetAllowedMounts([]string{allowed})
			body := `{"serverId":"` + testServerID + `","image":"busybox","mounts":[{"source":"` + test.source + `","target":"/mnt/data","read_only":true}]}`
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(body)))
			if rec.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, rec.Code, rec.Body.String())
			}
			if test.status == http.StatusAccepted && (len(rt.createReq.Mounts) != 1 || rt.createReq.Mounts[0].Source != allowedChild) {
				t.Fatalf("unexpected runtime mounts: %#v", rt.createReq.Mounts)
			}
		})
	}
}

func TestRuntimeRequestFromConfigurationParsesAndValidatesMounts(t *testing.T) {
	dataDir := t.TempDir()
	allowed := t.TempDir()
	server, _ := NewServer(&mountTestRuntime{}, dataDir)
	server.SetAllowedMounts([]string{allowed})
	if err := server.persistRuntimeRequest(testServerID, runtime.CreateRequest{ServerID: testServerID, Image: "busybox", RootDir: filepath.Join(dataDir, testServerID)}); err != nil {
		t.Fatal(err)
	}

	req, ok, err := server.runtimeRequestFromConfiguration(testServerID, map[string]any{
		"mounts": []any{map[string]any{"source": allowed, "target": "/mnt/data", "read_only": true}},
	})
	if err != nil || !ok {
		t.Fatalf("runtime request: ok=%v err=%v", ok, err)
	}
	if len(req.Mounts) != 1 || req.Mounts[0].Source != allowed || !req.Mounts[0].ReadOnly {
		t.Fatalf("unexpected mounts: %#v", req.Mounts)
	}

	_, _, err = server.runtimeRequestFromConfiguration(testServerID, map[string]any{
		"mounts": []any{map[string]any{"source": t.TempDir(), "target": "/mnt/data"}},
	})
	if err == nil || !strings.Contains(err.Error(), "allowed_mounts") {
		t.Fatalf("expected disallowed mount error, got %v", err)
	}
}

func TestConfigurationSyncReconcilesExistingWorkload(t *testing.T) {
	rt := &mountTestRuntime{exists: true}
	server, _ := NewServer(rt, t.TempDir())
	request := runtime.CreateRequest{ServerID: testServerID, Image: "busybox"}
	if err := server.reconcileRuntimeConfiguration(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if rt.createCalled || !rt.reconcileCalled {
		t.Fatalf("expected existing workload reconciliation, create=%v reconcile=%v", rt.createCalled, rt.reconcileCalled)
	}
}

type mountTestRuntime struct {
	stubRuntime
	exists          bool
	reconcileCalled bool
}

func (r *mountTestRuntime) Provider() string { return runtime.ProviderDocker }

func (r *mountTestRuntime) Inspect(context.Context, string) (runtime.ContainerState, error) {
	return runtime.ContainerState{Exists: r.exists}, nil
}

func (r *mountTestRuntime) Reconcile(_ context.Context, req runtime.CreateRequest) error {
	r.reconcileCalled = true
	r.createReq = req
	return nil
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o750)
}
