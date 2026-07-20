// Package storage owns persistent-volume and object-storage contracts. It is
// intentionally runtime-neutral so Docker volumes, S3-compatible storage, and
// future replicated storage can use the same workload attachments.
package storage

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module                                    { return Module{} }
func (Module) Name() string                          { return "storage" }
func (Module) WorkloadKinds() []workloads.Kind       { return nil }
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "storage.read", Description: "View volumes and object-storage configuration"}, {Key: "storage.write", Description: "Manage workload storage attachments"}}
}
func (Module) Routes() []modules.Route     { return nil }
func (Module) Start(context.Context) error { return nil }
