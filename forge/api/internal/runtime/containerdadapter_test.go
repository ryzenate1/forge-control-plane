package runtime

import (
	"context"
	"testing"
)

func TestNewContainerdAdapter(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	if adapter == nil {
		t.Fatal("NewContainerdAdapter(nil) should not return nil")
	}
}

func TestContainerdAdapterName(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	if got := adapter.Name(); got != ContainerdProvider {
		t.Errorf("Name() = %q, want %q", got, ContainerdProvider)
	}
}

func TestContainerdAdapterCapabilities(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	got := adapter.Capabilities()
	want := ContainerdCapabilities()
	if got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestContainerdAdapterProviderConstant(t *testing.T) {
	if ContainerdProvider != "containerd" {
		t.Errorf("ContainerdProvider = %q, want %q", ContainerdProvider, "containerd")
	}
}

func TestContainerdAdapterSupportsMigration(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	if adapter.SupportsMigration() {
		t.Error("SupportsMigration with nil client should be false")
	}
}

func TestContainerdAdapterPrepareMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	resp, err := adapter.PrepareMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("PrepareMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("PrepareMigration should not be accepted")
	}
	if resp.Mode != "not_implemented" {
		t.Errorf("PrepareMigration mode = %q, want %q", resp.Mode, "not_implemented")
	}
}

func TestContainerdAdapterExecuteMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	resp, err := adapter.ExecuteMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("ExecuteMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("ExecuteMigration should not be accepted")
	}
}

func TestContainerdAdapterCancelMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	resp, err := adapter.CancelMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("CancelMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("CancelMigration should not be accepted")
	}
}

func TestContainerdAdapterCreateServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.CreateServer(context.Background(), Target{}, CreateServerRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("CreateServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterDeleteServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.DeleteServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("DeleteServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterStartServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.StartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterStopServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.StopServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StopServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterRestartServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.RestartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("RestartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterKillServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.KillServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("KillServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterStatsNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.Stats(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Stats with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterExistsNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.Exists(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Exists with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterInspectNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.Inspect(context.Background(), Target{ServerID: "s1"})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Inspect with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterInstallServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.InstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("InstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterReinstallServerNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	_, err := adapter.ReinstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("ReinstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestContainerdAdapterSyncConfigurationNilClient(t *testing.T) {
	adapter := NewContainerdAdapter(nil)
	err := adapter.SyncServerConfiguration(context.Background(), Target{}, ServerConfiguration{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("SyncServerConfiguration with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}
