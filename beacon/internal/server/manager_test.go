package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gamepanel/beacon/internal/runtime"
)

type blockingRuntime struct {
	started      chan struct{}
	release      chan struct{}
	mu           sync.Mutex
	starts       int
	stops        int
	creates      int
	deletes      int
	inspectState map[string]runtime.ContainerState
}

func newBlockingRuntime() *blockingRuntime {
	return &blockingRuntime{started: make(chan struct{}, 8), release: make(chan struct{})}
}

func (r *blockingRuntime) Create(context.Context, runtime.CreateRequest) error {
	r.mu.Lock()
	r.creates++
	r.mu.Unlock()
	return nil
}
func (r *blockingRuntime) Install(context.Context, runtime.InstallRequest) (runtime.InstallResult, error) {
	return runtime.InstallResult{}, nil
}
func (r *blockingRuntime) Inspect(_ context.Context, serverID string) (runtime.ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inspectState != nil {
		return r.inspectState[serverID], nil
	}
	return runtime.ContainerState{ServerID: serverID, Exists: true}, nil
}
func (r *blockingRuntime) List(context.Context) ([]runtime.ContainerState, error) { return nil, nil }
func (r *blockingRuntime) Start(context.Context, string) error {
	r.mu.Lock()
	r.starts++
	r.mu.Unlock()
	r.started <- struct{}{}
	<-r.release
	return nil
}
func (r *blockingRuntime) SendCommand(context.Context, string, string) error { return nil }
func (r *blockingRuntime) Stop(context.Context, string) error {
	r.mu.Lock()
	r.stops++
	r.mu.Unlock()
	return nil
}
func (r *blockingRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	return r.Stop(ctx, serverID)
}
func (r *blockingRuntime) Kill(context.Context, string) error           { return nil }
func (r *blockingRuntime) Signal(context.Context, string, string) error { return nil }
func (r *blockingRuntime) Restart(context.Context, string) error        { return nil }
func (r *blockingRuntime) Stats(context.Context, string) (runtime.Stats, error) {
	return runtime.Stats{}, nil
}
func (r *blockingRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *blockingRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *blockingRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *blockingRuntime) AttachConsole(context.Context, string) (runtime.ConsoleSession, error) {
	return nil, nil
}
func (r *blockingRuntime) Delete(context.Context, string) error {
	r.mu.Lock()
	r.deletes++
	r.mu.Unlock()
	return nil
}

func (r *blockingRuntime) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts, r.stops
}

func TestServerManagerReconstructsActualContainerStates(t *testing.T) {
	rt := newBlockingRuntime()
	rt.inspectState = map[string]runtime.ContainerState{
		"running": {ServerID: "running", Exists: true, Running: true},
		"stopped": {ServerID: "stopped", Exists: true, Running: false},
		"missing": {ServerID: "missing", Exists: false},
	}
	manager := NewServerManager(rt)
	root := t.TempDir()
	for _, id := range []string{"running", "stopped", "missing"} {
		if err := manager.Reconcile(context.Background(), Reconstruction{
			ServerID: id, RootDir: filepath.Join(root, id), DiskLimitMB: 10,
			ConfigurationSynced: true, InstallationState: "installed",
		}); err != nil {
			t.Fatalf("reconcile %s: %v", id, err)
		}
	}
	for id, expected := range map[string]PowerState{
		"running": PowerStateRunning, "stopped": PowerStateOffline, "missing": PowerStateOffline,
	} {
		state := manager.State(id)
		if state.PowerState != expected {
			t.Fatalf("%s power state = %s, want %s", id, state.PowerState, expected)
		}
		if state.RootDir != filepath.Join(root, id) || !state.ConfigurationSynced || state.DiskLimitBytes != 10*1024*1024 {
			t.Fatalf("%s was not fully reconstructed: %+v", id, state)
		}
	}
	if !manager.State("running").ContainerExists || !manager.State("stopped").ContainerExists || manager.State("missing").ContainerExists {
		t.Fatal("container existence was not reconstructed from runtime inspection")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.creates != 0 || rt.deletes != 0 {
		t.Fatalf("reconstruction mutated containers: creates=%d deletes=%d", rt.creates, rt.deletes)
	}
}

func TestServerManagerRejectsConcurrentPowerActions(t *testing.T) {
	rt := newBlockingRuntime()
	manager := NewServerManager(rt)
	root := t.TempDir()
	manager.MarkCreated("demo", root, 0)
	manager.MarkConfigurationSynced("demo", -1)

	done := make(chan error, 1)
	go func() {
		done <- manager.HandlePower(context.Background(), "demo", "start")
	}()
	<-rt.started

	if err := manager.HandlePower(context.Background(), "demo", "stop"); err == nil {
		t.Fatal("expected concurrent power action to be rejected")
	}

	close(rt.release)
	if err := <-done; err != nil {
		t.Fatalf("expected first action to succeed: %v", err)
	}
}

func TestServerManagerRestartsUnexpectedCrash(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	manager := NewServerManager(rt)
	root := t.TempDir()
	manager.MarkCreated("demo", root, 0)
	manager.MarkConfigurationSynced("demo", -1)

	manager.HandleContainerEvent(context.Background(), runtime.ContainerEvent{
		ServerID: "demo",
		Action:   "die",
		ExitCode: 1,
	})

	starts, _ := rt.counts()
	if starts != 1 {
		t.Fatalf("expected crash restart, got %d starts", starts)
	}
}

func TestServerManagerDoesNotRestartExpectedStop(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	manager := NewServerManager(rt)
	root := t.TempDir()
	manager.MarkCreated("demo", root, 0)
	manager.MarkConfigurationSynced("demo", -1)
	state := manager.State("demo")
	state.ExpectedStop = true

	manager.HandleContainerEvent(context.Background(), runtime.ContainerEvent{
		ServerID: "demo",
		Action:   "die",
		ExitCode: 1,
	})

	starts, _ := rt.counts()
	if starts != 0 {
		t.Fatalf("expected no restart for expected stop, got %d starts", starts)
	}
}

func TestServerManagerCrashCooldownSuppressesRestart(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	manager := NewServerManager(rt)
	root := t.TempDir()
	manager.MarkCreated("demo", root, 0)
	manager.MarkConfigurationSynced("demo", -1)
	state := manager.State("demo")
	state.LastCrash = time.Now()
	state.CrashCooldown = time.Minute

	manager.HandleContainerEvent(context.Background(), runtime.ContainerEvent{
		ServerID: "demo",
		Action:   "die",
		ExitCode: 1,
	})

	starts, _ := rt.counts()
	if starts != 0 {
		t.Fatalf("expected crash cooldown to suppress restart, got %d starts", starts)
	}
}

func TestServerManagerBlocksStartBeforeConfigurationSync(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	manager := NewServerManager(rt)
	manager.MarkCreated("demo", t.TempDir(), 0)

	if err := manager.HandlePower(context.Background(), "demo", "start"); err == nil {
		t.Fatal("expected start to fail before configuration sync")
	}
}

func TestServerManagerBlocksStartWhenDiskLimitExceeded(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	manager := NewServerManager(rt)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.dat"), []byte(strings.Repeat("x", 2*1024*1024)), 0o640); err != nil {
		t.Fatal(err)
	}
	manager.MarkCreated("demo", root, 1)
	manager.MarkConfigurationSynced("demo", -1)

	if err := manager.HandlePower(context.Background(), "demo", "start"); err == nil {
		t.Fatal("expected start to fail when disk usage exceeds limit")
	}
}
