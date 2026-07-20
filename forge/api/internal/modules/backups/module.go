// Package backups owns backup-artifact and restore-operation contracts. The
// current backup service is retained as the storage adapter during migration.
package backups

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module                                    { return Module{} }
func (Module) Name() string                          { return "backups" }
func (Module) WorkloadKinds() []workloads.Kind       { return nil }
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "backups.read", Description: "View backup artifacts and policies"}, {Key: "backups.write", Description: "Create, restore, and manage backups"}}
}
func (Module) Routes() []modules.Route {
	return []modules.Route{{Method: "GET", Path: "/servers/:id/backups", Audience: "client"}, {Method: "POST", Path: "/servers/:id/backups", Audience: "client"}, {Method: "POST", Path: "/servers/:id/backups/cleanup", Audience: "client"}}
}
func (Module) Start(context.Context) error { return nil }
