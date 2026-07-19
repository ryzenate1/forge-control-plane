package runtime

import (
	"context"
	"testing"
)

func TestNewKubernetesAdapter(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	if adapter == nil {
		t.Fatal("NewKubernetesAdapter(nil) should not return nil")
	}
}

func TestKubernetesAdapterName(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	if got := adapter.Name(); got != KubernetesProvider {
		t.Errorf("Name() = %q, want %q", got, KubernetesProvider)
	}
}

func TestKubernetesAdapterCapabilities(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	got := adapter.Capabilities()
	want := KubernetesCapabilities()
	if got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestKubernetesAdapterProviderConstant(t *testing.T) {
	if KubernetesProvider != "kubernetes" {
		t.Errorf("KubernetesProvider = %q, want %q", KubernetesProvider, "kubernetes")
	}
}

func TestKubernetesAdapterSupportsMigration(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	if adapter.SupportsMigration() {
		t.Error("SupportsMigration with nil client should be false")
	}
}

func TestKubernetesAdapterPrepareMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
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

func TestKubernetesAdapterExecuteMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	resp, err := adapter.ExecuteMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("ExecuteMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("ExecuteMigration should not be accepted")
	}
}

func TestKubernetesAdapterCancelMigrationReturnsErrNotImplemented(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	resp, err := adapter.CancelMigration(context.Background(), MigrationRequest{MigrationID: "m1"})
	if err != ErrNotImplemented {
		t.Errorf("CancelMigration err = %v, want %v", err, ErrNotImplemented)
	}
	if resp.Accepted {
		t.Error("CancelMigration should not be accepted")
	}
}

func TestKubernetesAdapterCreateServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.CreateServer(context.Background(), Target{}, CreateServerRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("CreateServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterDeleteServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.DeleteServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("DeleteServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterStartServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.StartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterStopServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.StopServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("StopServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterRestartServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.RestartServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("RestartServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterKillServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.KillServer(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("KillServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterStatsNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.Stats(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Stats with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterExistsNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.Exists(context.Background(), Target{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Exists with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterInspectNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.Inspect(context.Background(), Target{ServerID: "s1"})
	if err != ErrRuntimeUnavailable {
		t.Errorf("Inspect with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterInstallServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.InstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("InstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterReinstallServerNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	_, err := adapter.ReinstallServer(context.Background(), Target{}, InstallRequest{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("ReinstallServer with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}

func TestKubernetesAdapterSyncConfigurationNilClient(t *testing.T) {
	adapter := NewKubernetesAdapter(nil)
	err := adapter.SyncServerConfiguration(context.Background(), Target{}, ServerConfiguration{})
	if err != ErrRuntimeUnavailable {
		t.Errorf("SyncServerConfiguration with nil client err = %v, want %v", err, ErrRuntimeUnavailable)
	}
}
