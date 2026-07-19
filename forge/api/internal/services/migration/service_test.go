package migration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"
)

func TestExecuteMigrationReturnsTypedNotImplementedWithoutStore(t *testing.T) {
	service := New(nil, nil, nil, nil, nil)

	_, err := service.ExecuteMigration(context.Background(), "migration-1")
	if err == nil {
		t.Fatal("expected migration execution to be unavailable")
	}
	var notImplemented *NotImplementedError
	if !errors.As(err, &notImplemented) {
		t.Fatalf("expected *NotImplementedError, got %T", err)
	}
	if notImplemented.MigrationID != "migration-1" {
		t.Fatalf("unexpected migration id %q", notImplemented.MigrationID)
	}
	if !errors.Is(err, gpruntime.ErrNotImplemented) {
		t.Fatalf("expected error to wrap runtime.ErrNotImplemented: %v", err)
	}
}

func TestPrepareMigrationReturnsTypedNotImplementedWithoutStore(t *testing.T) {
	service := New(nil, nil, nil, nil, nil)

	_, err := service.PrepareMigration(context.Background(), "migration-1")
	var notImplemented *NotImplementedError
	if !errors.As(err, &notImplemented) {
		t.Fatalf("expected *NotImplementedError, got %T (%v)", err, err)
	}
}

func TestNewCredential(t *testing.T) {
	cred, err := newCredential()
	if err != nil {
		t.Fatalf("newCredential() returned error: %v", err)
	}
	if len(cred) != 64 {
		t.Fatalf("expected credential of length 64 (32 hex-encoded bytes), got %d", len(cred))
	}
}

func TestNewCredentialUnique(t *testing.T) {
	a, _ := newCredential()
	b, _ := newCredential()
	if a == b {
		t.Fatal("expected successive credentials to be unique")
	}
}

func TestCredentialHash(t *testing.T) {
	cred := "abc123"
	hash := credentialHash(cred)
	if len(hash) != 64 {
		t.Fatalf("expected sha256 hex of length 64, got %d", len(hash))
	}
	if credentialHash(cred) != credentialHash(cred) {
		t.Fatal("expected credentialHash to be deterministic")
	}
}

func TestRandomID(t *testing.T) {
	id := randomID()
	if id == "" {
		t.Fatal("expected non-empty worker id")
	}
}

func TestPhaseAtLeast(t *testing.T) {
	tests := []struct {
		current  string
		expected string
		want     bool
	}{
		{"planned", "planned", true},
		{"planned", "source_archived", false},
		{"source_archived", "planned", true},
		{"source_archived", "source_archived", true},
		{"destination_created", "destination_created", true},
		{"destination_created", "completed", false},
		{"completed", "planned", true},
		{"completed", "completed", true},
		{"", "planned", true},
		{"credentials_registered", "source_archived", false},
		{"source_archived", "credentials_registered", true},
	}
	for _, tt := range tests {
		got := phaseAtLeast(tt.current, tt.expected)
		if got != tt.want {
			t.Errorf("phaseAtLeast(%q, %q) = %v, want %v", tt.current, tt.expected, got, tt.want)
		}
	}
}

func TestRuntimeTarget(t *testing.T) {
	target := runtimeTarget(store.ServerProvisionTarget{
		NodeURL:   "https://node.example.com:8080",
		NodeToken: "tok-abc",
		ServerID:  "srv-001",
	})
	if target.NodeURL != "https://node.example.com:8080" {
		t.Errorf("unexpected NodeURL: %s", target.NodeURL)
	}
	if target.NodeToken != "tok-abc" {
		t.Errorf("unexpected NodeToken: %s", target.NodeToken)
	}
	if target.ServerID != "srv-001" {
		t.Errorf("unexpected ServerID: %s", target.ServerID)
	}
	if target.NodeID != "" {
		t.Errorf("expected empty NodeID, got %q", target.NodeID)
	}
}

func TestRuntimeCreateRequest(t *testing.T) {
	req := runtimeCreateRequest(store.ServerProvisionTarget{
		ServerID:      "srv-001",
		Name:          "my-server",
		Image:         "ubuntu:22.04",
		MemoryMB:      2048,
		SwapMB:        512,
		CPUShares:     1024,
		CPULimit:      2048,
		DiskMB:        10000,
		IOWeight:      500,
		Threads:       "2",
		OOMDisabled:   false,
		AllocationIP:  "10.0.0.1",
		AllocationPort: 25565,
		Environment:   map[string]string{"FOO": "bar"},
		Allocations:   nil,
		Mounts:        nil,
		StartupCommand: "./start.sh",
	})
	if req.ServerID != "srv-001" {
		t.Errorf("unexpected ServerID: %s", req.ServerID)
	}
	if req.Name != "my-server" {
		t.Errorf("unexpected Name: %s", req.Name)
	}
	if req.Image != "ubuntu:22.04" {
		t.Errorf("unexpected Image: %s", req.Image)
	}
	if req.MemoryMB != 2048 {
		t.Errorf("unexpected MemoryMB: %d", req.MemoryMB)
	}
	if req.CPUShares != 1024 {
		t.Errorf("unexpected CPUShares: %d", req.CPUShares)
	}
	if !strings.Contains(strings.Join(req.Env, " "), "FOO=bar") {
		t.Errorf("expected env to contain FOO=bar, got %v", req.Env)
	}
	if !strings.Contains(strings.Join(req.Env, " "), "SERVER_MEMORY=2048") {
		t.Errorf("expected env to contain SERVER_MEMORY=2048, got %v", req.Env)
	}
	if len(req.Command) != 3 || req.Command[0] != "/bin/sh" {
		t.Errorf("unexpected command: %v", req.Command)
	}
}

func TestRuntimeCreateRequestMinecraft(t *testing.T) {
	req := runtimeCreateRequest(store.ServerProvisionTarget{
		Image:         "itzg/minecraft-server:latest",
		StartupCommand: "./start.sh",
	})
	if req.Command != nil {
		t.Errorf("expected nil command for minecraft image, got %v", req.Command)
	}
	found := false
	for _, e := range req.Env {
		if e == "EULA=TRUE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected EULA=TRUE in env for minecraft image, got %v", req.Env)
	}
}

func TestRuntimeCreateRequestAllocationsAndMounts(t *testing.T) {
	req := runtimeCreateRequest(store.ServerProvisionTarget{
		ServerID: "srv-001",
		Allocations: []store.ServerRuntimeAllocation{
			{IP: "10.0.0.1", Port: 25565},
			{IP: "10.0.0.1", Port: 25566},
		},
		Mounts: []store.ServerMount{
			{Source: "/data", Target: "/mnt/data", ReadOnly: false},
		},
	})
	if len(req.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(req.Ports))
	}
	if req.Ports[0].HostPort != 25565 || req.Ports[0].ContainerPort != 25565 {
		t.Errorf("unexpected port mapping: %+v", req.Ports[0])
	}
	if len(req.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(req.Mounts))
	}
	if req.Mounts[0].Source != "/data" || req.Mounts[0].Target != "/mnt/data" {
		t.Errorf("unexpected mount: %+v", req.Mounts[0])
	}
}

func TestServiceMetricsNil(t *testing.T) {
	var s *Service
	m := s.Metrics()
	if m != (Metrics{}) {
		t.Errorf("expected zero Metrics from nil receiver, got %+v", m)
	}
}

func TestServiceMetricsInitialZero(t *testing.T) {
	s := New(nil, nil, nil, nil, nil)
	m := s.Metrics()
	if m.MigrationTotal != 0 || m.MigrationCompletedTotal != 0 || m.MigrationFailedTotal != 0 {
		t.Errorf("expected all counters to be zero, got %+v", m)
	}
}

func TestNotImplementedError(t *testing.T) {
	e := &NotImplementedError{Operation: "test", MigrationID: "mig-1"}
	if !errors.Is(e, gpruntime.ErrNotImplemented) {
		t.Error("expected NotImplementedError to wrap ErrNotImplemented")
	}
	msg := e.Error()
	if !strings.Contains(msg, "test") || !strings.Contains(msg, "not implemented") {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestStartShutdown(t *testing.T) {
	s := New(nil, nil, nil, nil, nil)
	s.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	s.Start(ctx)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

func TestShutdownNilService(t *testing.T) {
	var s *Service
	if err := s.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on nil service should return nil, got: %v", err)
	}
}

func TestStartNilService(t *testing.T) {
	var s *Service
	s.Start(context.Background())
}

func TestValidateMigrationNilStore(t *testing.T) {
	s := New(nil, nil, nil, nil, nil)
	_, err := s.ValidateMigration(context.Background(), CreateMigrationRequest{ServerID: "srv-1"})
	if err == nil {
		t.Fatal("expected error from nil store")
	}
	if err.Error() != "store unavailable" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateMigrationEmptyServerID(t *testing.T) {
	s := &Service{store: &store.Store{}}
	_, err := s.ValidateMigration(context.Background(), CreateMigrationRequest{})
	if err == nil {
		t.Fatal("expected error for empty serverId")
	}
	if err.Error() != "serverId is required" {
		t.Errorf("unexpected error: %v", err)
	}
}
