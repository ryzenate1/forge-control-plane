package apphosting

import (
	"context"

	"gamepanel/forge/internal/platform/modules"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Module struct{}

func New() Module           { return Module{} }
func (Module) Name() string { return "apphosting" }
func (Module) WorkloadKinds() []workloads.Kind {
	return []workloads.Kind{workloads.KindApplication, workloads.KindComposeStack, workloads.KindStaticSite, workloads.KindBackgroundWorker, workloads.KindCronJob}
}
func (Module) OperationDrivers() []operations.Driver { return nil }
func (Module) Permissions() []modules.Permission {
	return []modules.Permission{{Key: "apps.read", Description: "View applications"}, {Key: "apps.write", Description: "Create and deploy applications"}}
}
func (Module) Routes() []modules.Route {
	return []modules.Route{{Method: "GET", Path: "/platform/workloads", Audience: "provider"}, {Method: "POST", Path: "/platform/workloads", Audience: "provider"}, {Method: "POST", Path: "/platform/applications", Audience: "provider"}}
}
func (Module) Start(context.Context) error { return nil }
