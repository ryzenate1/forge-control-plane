package gameservers

import (
	"context"
	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module                                    { return Module{} }
func (Module) Name() string                          { return "gameservers" }
func (Module) WorkloadKinds() []workloads.Kind       { return []workloads.Kind{workloads.KindGameServer} }
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "gameservers.read", Description: "View game servers"}, {Key: "gameservers.write", Description: "Create and configure game servers"}, {Key: "gameservers.power", Description: "Control game-server power"}}
}
func (Module) Routes() []modules.Route {
	return []modules.Route{{Method: "GET", Path: "/servers", Audience: "client"}, {Method: "POST", Path: "/servers", Audience: "client"}, {Method: "POST", Path: "/servers/:id/power", Audience: "client"}}
}
func (Module) Start(context.Context) error { return nil }
