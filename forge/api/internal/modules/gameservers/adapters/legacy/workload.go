package legacy

import (
	legacydomain "gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/platform/tenancy"
	"gamepanel/forge/internal/platform/workloads"
	"time"
)

// WorkloadFromServer is the compatibility bridge used while existing server
// rows are migrated to the canonical workload model.
func WorkloadFromServer(server legacydomain.Server) workloads.Workload {
	scope := tenancy.DefaultScope()
	now := time.Now().UTC()
	return workloads.Workload{ID: server.ID, EnvironmentID: scope.EnvironmentID, Kind: workloads.KindGameServer, Name: server.Name,
		DesiredGeneration: 1, ObservedGeneration: 0, DesiredState: workloads.DesiredState(server.DesiredState), ObservedState: workloads.ObservedState(server.ActualState), CreatedAt: now, UpdatedAt: now}
}
