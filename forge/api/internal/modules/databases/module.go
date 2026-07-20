// Package databases owns the control-plane contract for database and cache
// workloads. The existing database-host provisioner remains the runtime
// adapter while its command driver is moved behind this module incrementally.
package databases

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module           { return Module{} }
func (Module) Name() string { return "databases" }
func (Module) WorkloadKinds() []workloads.Kind {
	return []workloads.Kind{workloads.KindDatabase, workloads.KindCache}
}
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{
		{Key: "databases.read", Description: "View database hosts and credentials"},
		{Key: "databases.write", Description: "Configure and provision databases"},
		{Key: "databases.delete", Description: "Remove database hosts"},
	}
}
func (Module) Routes() []modules.Route {
	return []modules.Route{
		{Method: "GET", Path: "/database-hosts", Audience: "provider"},
		{Method: "POST", Path: "/database-hosts", Audience: "provider"},
		{Method: "POST", Path: "/servers/:id/databases", Audience: "client"},
	}
}
func (Module) Start(context.Context) error { return nil }
