package orchestrator

import "gamepanel/forge/internal/store"

type RegionCapacity struct {
	RegionID        string                       `json:"regionId"`
	Nodes           []store.NodeCapacitySnapshot `json:"nodes"`
	AllocatedCPU    int                          `json:"allocated_cpu"`
	AvailableCPU    int                          `json:"available_cpu"`
	AllocatedMemory int                          `json:"allocated_memory"`
	AvailableMemory int                          `json:"available_memory"`
	AllocatedDisk   int                          `json:"allocated_disk"`
	AvailableDisk   int                          `json:"available_disk"`
	ServerCount     int                          `json:"server_count"`
}

func ComputeRegionCapacity(regionID string, snapshots []store.NodeCapacitySnapshot) RegionCapacity {
	capacity := RegionCapacity{RegionID: regionID, Nodes: snapshots}
	for _, snapshot := range snapshots {
		capacity.AllocatedCPU += snapshot.AllocatedCPU
		capacity.AvailableCPU += snapshot.AvailableCPU
		capacity.AllocatedMemory += snapshot.AllocatedMemory
		capacity.AvailableMemory += snapshot.AvailableMemory
		capacity.AllocatedDisk += snapshot.AllocatedDisk
		capacity.AvailableDisk += snapshot.AvailableDisk
		capacity.ServerCount += snapshot.ServerCount
	}
	return capacity
}
