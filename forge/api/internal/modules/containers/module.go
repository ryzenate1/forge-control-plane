// Package containers groups generic Docker, system-container, and VM
// workloads without coupling core scheduling to any individual runtime.
package containers

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module           { return Module{} }
func (Module) Name() string { return "containers" }
func (Module) WorkloadKinds() []workloads.Kind {
	return []workloads.Kind{workloads.KindGenericContainer, workloads.KindSystemContainer, workloads.KindVirtualMachine}
}
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "containers.read", Description: "View runtime containers and images"}, {Key: "containers.write", Description: "Manage generic containers and system workloads"}}
}
func (Module) Routes() []modules.Route     { return nil }
func (Module) Start(context.Context) error { return nil }
