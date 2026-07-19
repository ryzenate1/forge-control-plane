package runtime

import (
	"context"
	"errors"
	"testing"
)

type mockRuntime struct {
	name             string
	caps             Capabilities
	supportsMigrate  bool
	reinstallErr     error
	reinstallSupport bool
}

func (m *mockRuntime) Name() string                                 { return m.name }
func (m *mockRuntime) Capabilities() Capabilities                   { return m.caps }
func (m *mockRuntime) SupportsMigration() bool                      { return m.supportsMigrate }
func (m *mockRuntime) CreateServer(context.Context, Target, CreateServerRequest) (CreateResponse, error) {
	return CreateResponse{}, nil
}
func (m *mockRuntime) InstallServer(context.Context, Target, InstallRequest) (InstallResponse, error) {
	return InstallResponse{}, nil
}
func (m *mockRuntime) SyncServerConfiguration(context.Context, Target, ServerConfiguration) error { return nil }
func (m *mockRuntime) ResizeServer(context.Context, Target, int64, int64) error                   { return nil }
func (m *mockRuntime) DeleteServer(context.Context, Target) (PowerResponse, error)                 { return PowerResponse{}, nil }
func (m *mockRuntime) StartServer(context.Context, Target) (PowerResponse, error)                  { return PowerResponse{}, nil }
func (m *mockRuntime) StopServer(context.Context, Target) (PowerResponse, error)                   { return PowerResponse{}, nil }
func (m *mockRuntime) RestartServer(context.Context, Target) (PowerResponse, error)                { return PowerResponse{}, nil }
func (m *mockRuntime) KillServer(context.Context, Target) (PowerResponse, error)                   { return PowerResponse{}, nil }
func (m *mockRuntime) Stats(context.Context, Target) (Stats, error)                                { return Stats{}, nil }
func (m *mockRuntime) Exists(context.Context, Target) (bool, error)                                { return true, nil }
func (m *mockRuntime) Inspect(context.Context, Target) (Inspection, error)                         { return Inspection{}, nil }
func (m *mockRuntime) PrepareMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, ErrNotImplemented
}
func (m *mockRuntime) ExecuteMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, ErrNotImplemented
}
func (m *mockRuntime) CancelMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, ErrNotImplemented
}

func (m *mockRuntime) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	if !m.reinstallSupport {
		return InstallResponse{}, ErrNotImplemented
	}
	if m.reinstallErr != nil {
		return InstallResponse{}, m.reinstallErr
	}
	return InstallResponse{ServerID: req.ServerID, Accepted: true}, nil
}

func TestNewMultiRuntimeAdapter(t *testing.T) {
	adapter := NewMultiRuntimeAdapter(nil)
	if adapter == nil {
		t.Fatal("NewMultiRuntimeAdapter should not return nil")
	}
	if adapter.Name() != "multi-runtime" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "multi-runtime")
	}
}

func TestMultiRuntimeAdapterRegisterAndGetRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	mr := &mockRuntime{name: "test"}
	m.Register("test", mr)
	got, ok := m.GetRuntime("test")
	if !ok {
		t.Fatal("GetRuntime should find registered runtime")
	}
	if got != mr {
		t.Error("GetRuntime should return the same instance")
	}
}

func TestMultiRuntimeAdapterRegisterNilRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", nil)
	_, ok := m.GetRuntime("test")
	if ok {
		t.Error("Register nil runtime should not store it")
	}
}

func TestMultiRuntimeAdapterRegisterEmptyProvider(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	m.Register("", &mockRuntime{name: "x"})
	_, ok := m.GetRuntime("")
	if ok {
		t.Error("Register empty provider should not store it")
	}
}

func TestMultiRuntimeAdapterGetRuntimeNotFound(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, ok := m.GetRuntime("nonexistent")
	if ok {
		t.Error("GetRuntime for nonexistent provider should return false")
	}
}

func TestMultiRuntimeAdapterGetRuntimeForTargetUsesProvider(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	specificRt := &mockRuntime{name: DockerProvider}
	m := NewMultiRuntimeAdapter(defaultRt)
	m.Register(DockerProvider, specificRt)
	target := Target{Provider: DockerProvider}
	rt := m.getRuntimeForTarget(target)
	if rt != specificRt {
		t.Error("getRuntimeForTarget should return the specific runtime for the provider")
	}
}

func TestMultiRuntimeAdapterGetRuntimeForTargetFallsBackToDefault(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	m := NewMultiRuntimeAdapter(defaultRt)
	target := Target{Provider: "unknown"}
	rt := m.getRuntimeForTarget(target)
	if rt != defaultRt {
		t.Error("getRuntimeForTarget should fall back to default for unknown provider")
	}
}

func TestMultiRuntimeAdapterGetRuntimeForTargetEmptyProvider(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	m := NewMultiRuntimeAdapter(defaultRt)
	target := Target{Provider: ""}
	rt := m.getRuntimeForTarget(target)
	if rt != defaultRt {
		t.Error("getRuntimeForTarget should fall back to default for empty provider")
	}
}

func TestMultiRuntimeAdapterGetRuntimeForTargetNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	target := Target{Provider: ""}
	rt := m.getRuntimeForTarget(target)
	if rt != nil {
		t.Error("getRuntimeForTarget should return nil when no default and no match")
	}
}

func TestMultiRuntimeAdapterCapabilitiesUnion(t *testing.T) {
	defaultRt := &mockRuntime{name: "default", caps: Capabilities{Containers: true}}
	specificRt := &mockRuntime{name: DockerProvider, caps: Capabilities{ResourceLimits: true}}
	m := NewMultiRuntimeAdapter(defaultRt)
	m.Register(DockerProvider, specificRt)
	caps := m.Capabilities()
	if !caps.Containers || !caps.ResourceLimits {
		t.Error("Capabilities should union both runtimes' capabilities")
	}
}

func TestMultiRuntimeAdapterCapabilitiesNoDefault(t *testing.T) {
	rt := &mockRuntime{name: "test", caps: Capabilities{MicroVM: true}}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	caps := m.Capabilities()
	if !caps.MicroVM {
		t.Error("Capabilities should include registered runtime's capabilities")
	}
}

func TestMultiRuntimeAdapterCapabilitiesEmpty(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	caps := m.Capabilities()
	if caps != (Capabilities{}) {
		t.Errorf("Capabilities should be zero value when no runtimes registered, got %+v", caps)
	}
}

func TestMultiRuntimeAdapterSupportsMigrationAny(t *testing.T) {
	m := NewMultiRuntimeAdapter(&mockRuntime{name: "a", supportsMigrate: false})
	m.Register("b", &mockRuntime{name: "b", supportsMigrate: true})
	if !m.SupportsMigration() {
		t.Error("SupportsMigration should return true when any runtime supports it")
	}
}

func TestMultiRuntimeAdapterSupportsMigrationDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(&mockRuntime{name: "a", supportsMigrate: true})
	if !m.SupportsMigration() {
		t.Error("SupportsMigration should return true when default supports it")
	}
}

func TestMultiRuntimeAdapterSupportsMigrationNone(t *testing.T) {
	m := NewMultiRuntimeAdapter(&mockRuntime{name: "a", supportsMigrate: false})
	if m.SupportsMigration() {
		t.Error("SupportsMigration should return false when none support it")
	}
}

func TestMultiRuntimeAdapterSupportsMigrationNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	if m.SupportsMigration() {
		t.Error("SupportsMigration should return false with no default and no registered runtimes")
	}
}

func TestMultiRuntimeAdapterCreateServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.CreateServer(context.Background(), Target{Provider: "test"}, CreateServerRequest{})
	if err != nil {
		t.Errorf("CreateServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterCreateServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.CreateServer(context.Background(), Target{}, CreateServerRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("CreateServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterInstallServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.InstallServer(context.Background(), Target{Provider: "test"}, InstallRequest{})
	if err != nil {
		t.Errorf("InstallServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterInstallServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.InstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("InstallServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterReinstallServerDelegates(t *testing.T) {
	mr := &mockRuntime{name: "test", reinstallSupport: true}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", mr)
	resp, err := m.ReinstallServer(context.Background(), Target{Provider: "test"}, InstallRequest{ServerID: "s1"})
	if err != nil {
		t.Errorf("ReinstallServer should delegate, got err: %v", err)
	}
	if !resp.Accepted {
		t.Error("ReinstallServer should return accepted response from runtime")
	}
}

func TestMultiRuntimeAdapterReinstallServerNoReinstaller(t *testing.T) {
	mr := &mockRuntime{name: "test", reinstallSupport: false}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", mr)
	_, err := m.ReinstallServer(context.Background(), Target{Provider: "test"}, InstallRequest{})
	if err != ErrNotImplemented {
		t.Errorf("ReinstallServer without Reinstaller err = %v, want %v", err, ErrNotImplemented)
	}
}

func TestMultiRuntimeAdapterReinstallServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.ReinstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("ReinstallServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterSyncConfigurationDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	err := m.SyncServerConfiguration(context.Background(), Target{Provider: "test"}, ServerConfiguration{})
	if err != nil {
		t.Errorf("SyncServerConfiguration should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterSyncConfigurationNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	err := m.SyncServerConfiguration(context.Background(), Target{}, ServerConfiguration{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("SyncServerConfiguration with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterDeleteServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.DeleteServer(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("DeleteServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterDeleteServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.DeleteServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("DeleteServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterStartServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.StartServer(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("StartServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterStartServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.StartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StartServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterStopServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.StopServer(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("StopServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterStopServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.StopServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StopServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterRestartServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.RestartServer(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("RestartServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterRestartServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.RestartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("RestartServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterKillServerDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.KillServer(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("KillServer should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterKillServerNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.KillServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("KillServer with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterStatsDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.Stats(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("Stats should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterStatsNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.Stats(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Stats with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterExistsDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	exists, err := m.Exists(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("Exists should delegate, got err: %v", err)
	}
	if !exists {
		t.Error("Exists should return runtime's result")
	}
}

func TestMultiRuntimeAdapterExistsNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.Exists(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Exists with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterInspectDelegates(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(nil)
	m.Register("test", rt)
	_, err := m.Inspect(context.Background(), Target{Provider: "test"})
	if err != nil {
		t.Errorf("Inspect should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterInspectNoRuntime(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.Inspect(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Inspect with no runtime err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestMultiRuntimeAdapterPrepareMigrationDelegatesToDefault(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	m := NewMultiRuntimeAdapter(defaultRt)
	_, err := m.PrepareMigration(context.Background(), MigrationRequest{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("PrepareMigration should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterPrepareMigrationNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.PrepareMigration(context.Background(), MigrationRequest{})
	if err != ErrNotImplemented {
		t.Errorf("PrepareMigration with no default err = %v, want %v", err, ErrNotImplemented)
	}
}

func TestMultiRuntimeAdapterExecuteMigrationDelegatesToDefault(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	m := NewMultiRuntimeAdapter(defaultRt)
	_, err := m.ExecuteMigration(context.Background(), MigrationRequest{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("ExecuteMigration should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterExecuteMigrationNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.ExecuteMigration(context.Background(), MigrationRequest{})
	if err != ErrNotImplemented {
		t.Errorf("ExecuteMigration with no default err = %v, want %v", err, ErrNotImplemented)
	}
}

func TestMultiRuntimeAdapterCancelMigrationDelegatesToDefault(t *testing.T) {
	defaultRt := &mockRuntime{name: "default"}
	m := NewMultiRuntimeAdapter(defaultRt)
	_, err := m.CancelMigration(context.Background(), MigrationRequest{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("CancelMigration should delegate, got err: %v", err)
	}
}

func TestMultiRuntimeAdapterCancelMigrationNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	_, err := m.CancelMigration(context.Background(), MigrationRequest{})
	if err != ErrNotImplemented {
		t.Errorf("CancelMigration with no default err = %v, want %v", err, ErrNotImplemented)
	}
}

func TestMultiRuntimeAdapterTransferClientWithoutTransferClient(t *testing.T) {
	rt := &mockRuntime{name: "test"}
	m := NewMultiRuntimeAdapter(rt)
	client := m.TransferClient()
	if client != nil {
		t.Error("TransferClient should return nil when runtime has no TransferClient method")
	}
}

func TestMultiRuntimeAdapterTransferClientNoDefault(t *testing.T) {
	m := NewMultiRuntimeAdapter(nil)
	client := m.TransferClient()
	if client != nil {
		t.Error("TransferClient should return nil when no default runtime")
	}
}
