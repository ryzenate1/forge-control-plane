package bootstrap

import (
	"testing"

	"gamepanel/forge/internal/platform/workloads"
)

func TestModuleRegistryOwnsEveryBuiltInWorkloadKind(t *testing.T) {
	registry, err := ModuleRegistry()
	if err != nil {
		t.Fatal(err)
	}
	wantOwners := map[workloads.Kind]string{
		workloads.KindApplication:      "apphosting",
		workloads.KindGameServer:       "gameservers",
		workloads.KindDatabase:         "databases",
		workloads.KindCache:            "databases",
		workloads.KindGenericContainer: "containers",
		workloads.KindSystemContainer:  "containers",
		workloads.KindVirtualMachine:   "containers",
	}
	for kind, want := range wantOwners {
		module, ok := registry.Owner(kind)
		if !ok || module.Name() != want {
			t.Fatalf("owner for %q = %v, want %q", kind, module, want)
		}
	}
	if len(registry.Modules()) != 7 {
		t.Fatalf("module count = %d, want 7", len(registry.Modules()))
	}
}
