package runtime

import (
	"context"
	"testing"
)

func TestNewFirecrackerAdapter(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	if adapter == nil {
		t.Fatal("NewFirecrackerAdapter(nil) should not return nil")
	}
}

func TestFirecrackerAdapterName(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	if got := adapter.Name(); got != FirecrackerProvider {
		t.Errorf("Name() = %q, want %q", got, FirecrackerProvider)
	}
}

func TestFirecrackerAdapterCapabilities(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	got := adapter.Capabilities()
	want := FirecrackerCapabilities()
	if got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestFirecrackerAdapterProviderConstant(t *testing.T) {
	if FirecrackerProvider != "firecracker" {
		t.Errorf("FirecrackerProvider = %q, want %q", FirecrackerProvider, "firecracker")
	}
}

func TestFirecrackerAdapterSupportsMigration(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	if adapter.SupportsMigration() {
		t.Error("SupportsMigration with nil client should be false")
	}
}

func TestFirecrackerAdapterPrepareMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
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

func TestFirecrackerAdapterExecuteMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	resp, err := adapter.ExecuteMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("ExecuteMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("ExecuteMigration should not be accepted")
	}
}

func TestFirecrackerAdapterCancelMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	resp, err := adapter.CancelMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("CancelMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("CancelMigration should not be accepted")
	}
}

func TestFirecrackerAdapterCreateServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.CreateServer(context.Background(), Target{}, CreateServerRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("CreateServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterDeleteServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.DeleteServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("DeleteServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterStartServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.StartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterStopServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.StopServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StopServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterRestartServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.RestartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("RestartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterKillServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.KillServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("KillServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterStatsNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.Stats(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Stats with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterExistsNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.Exists(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Exists with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterInspectNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.Inspect(context.Background(), Target{ServerID: "s1"})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Inspect with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterInstallServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.InstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("InstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterReinstallServerNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	_, err := adapter.ReinstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("ReinstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestFirecrackerAdapterSyncConfigurationNilClient(t *testing.T) {
	adapter := NewFirecrackerAdapter(nil)
	err := adapter.SyncServerConfiguration(context.Background(), Target{}, ServerConfiguration{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("SyncServerConfiguration with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}
