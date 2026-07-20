// Package networking owns routes, gateway policy, and private-network
// integration contracts. Existing traffic-manager and load-balancer services
// remain adapters until their persistence is migrated to platform routes.
package networking

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module                                    { return Module{} }
func (Module) Name() string                          { return "networking" }
func (Module) WorkloadKinds() []workloads.Kind       { return nil }
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "traffic.read", Description: "View routes and traffic policy"}, {Key: "traffic.write", Description: "Manage routes, gateways, and traffic policy"}}
}
func (Module) Routes() []modules.Route {
	return []modules.Route{{Method: "GET", Path: "/admin/traffic/rules", Audience: "provider"}, {Method: "POST", Path: "/admin/traffic/rules", Audience: "provider"}, {Method: "GET", Path: "/admin/load-balancer", Audience: "provider"}}
}
func (Module) Start(context.Context) error { return nil }
