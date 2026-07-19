package modules

import (
	"context"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
	"testing"
)

type testModule struct {
	name string
	kind workloads.Kind
}

func (m testModule) Name() string                        { return m.name }
func (m testModule) WorkloadKinds() []workloads.Kind     { return []workloads.Kind{m.kind} }
func (testModule) OperationDrivers() []operations.Driver { return nil }
func (testModule) Permissions() []Permission             { return nil }
func (testModule) Routes() []Route                       { return nil }
func (testModule) Start(context.Context) error           { return nil }

func TestRegistryRejectsDuplicateNamesAndKinds(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(testModule{"games", workloads.KindGameServer}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(testModule{"games", workloads.KindApplication}); err == nil {
		t.Fatal("expected duplicate name error")
	}
	if err := r.Register(testModule{"other", workloads.KindGameServer}); err == nil {
		t.Fatal("expected duplicate kind error")
	}
}
