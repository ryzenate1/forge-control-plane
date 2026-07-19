package runtime

import (
	"context"
	"testing"
)

func TestNewPodmanAdapter(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	if adapter == nil {
		t.Fatal("NewPodmanAdapter(nil) should not return nil")
	}
}

func TestPodmanAdapterName(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	if got := adapter.Name(); got != PodmanProvider {
		t.Errorf("Name() = %q, want %q", got, PodmanProvider)
	}
}

func TestPodmanAdapterCapabilities(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	got := adapter.Capabilities()
	want := PodmanCapabilities()
	if got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestPodmanAdapterProviderConstant(t *testing.T) {
	if PodmanProvider != "podman" {
		t.Errorf("PodmanProvider = %q, want %q", PodmanProvider, "podman")
	}
}

func TestPodmanAdapterSupportsMigration(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	if adapter.SupportsMigration() {
		t.Error("SupportsMigration with nil client should be false")
	}
}

func TestPodmanAdapterPrepareMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
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

func TestPodmanAdapterExecuteMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	resp, err := adapter.ExecuteMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("ExecuteMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("ExecuteMigration should not be accepted")
	}
}

func TestPodmanAdapterCancelMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	resp, err := adapter.CancelMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("CancelMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("CancelMigration should not be accepted")
	}
}

func TestPodmanAdapterCreateServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.CreateServer(context.Background(), Target{}, CreateServerRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("CreateServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterDeleteServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.DeleteServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("DeleteServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterStartServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.StartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterStopServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.StopServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StopServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterRestartServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.RestartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("RestartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterKillServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.KillServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("KillServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterStatsNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.Stats(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Stats with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterExistsNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.Exists(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Exists with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterInspectNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.Inspect(context.Background(), Target{ServerID: "s1"})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Inspect with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterInstallServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.InstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("InstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterReinstallServerNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	_, err := adapter.ReinstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("ReinstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestPodmanAdapterSyncConfigurationNilClient(t *testing.T) {
	adapter := NewPodmanAdapter(nil)
	err := adapter.SyncServerConfiguration(context.Background(), Target{}, ServerConfiguration{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("SyncServerConfiguration with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}
