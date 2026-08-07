package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type NodeCapacitySnapshot struct {
	NodeID          string    `json:"nodeId"`
	RegionID        string    `json:"regionId,omitempty"`
	TotalCPU        int       `json:"-"`
	TotalMemory     int       `json:"-"`
	TotalDisk       int       `json:"-"`
	AllocatedCPU    int       `json:"allocated_cpu"`
	AvailableCPU    int       `json:"available_cpu"`
	AllocatedMemory int       `json:"allocated_memory"`
	AvailableMemory int       `json:"available_memory"`
	AllocatedDisk   int       `json:"allocated_disk"`
	AvailableDisk   int       `json:"available_disk"`
	ServerCount     int       `json:"server_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) NodeCapacitySnapshot(ctx context.Context, nodeID string) (NodeCapacitySnapshot, error) {
	return s.nodeCapacitySnapshotTx(ctx, s.db, nodeID)
}

func (s *Store) LockNodeCapacitySnapshot(ctx context.Context, nodeID string) (NodeCapacitySnapshot, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	defer tx.Rollback(ctx)
	snapshot, err := s.lockedNodeCapacitySnapshotTx(ctx, tx, nodeID)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return NodeCapacitySnapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) nodeCapacitySnapshotTx(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, nodeID string) (NodeCapacitySnapshot, error) {
	var snapshot NodeCapacitySnapshot
	var regionID sql.NullString
	var cpuThreads, memoryMB, diskMB int
	err := querier.QueryRow(ctx, `
		SELECT n.id::text, n.region_id::text,
		       COALESCE(n.cpu_threads, 0),
		       COALESCE(NULLIF(n.node_memory_mb, 0), n.memory_mb, 0),
		       COALESCE(NULLIF(n.node_disk_mb, 0), n.disk_mb, 0),
		       COALESCE((SELECT SUM(s.cpu_shares) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.cpu) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.memory_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.memory_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.disk_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.disk_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT COUNT(s.id) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT COUNT(i.id) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       now()
		FROM nodes n
		WHERE n.id = $1
	`, nodeID).Scan(
		&snapshot.NodeID,
		&regionID,
		&cpuThreads,
		&memoryMB,
		&diskMB,
		&snapshot.AllocatedCPU,
		&snapshot.AllocatedMemory,
		&snapshot.AllocatedDisk,
		&snapshot.ServerCount,
		&snapshot.UpdatedAt,
	)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	if regionID.Valid {
		snapshot.RegionID = regionID.String
	}
	snapshot.TotalCPU = cpuThreads * 1024
	snapshot.TotalMemory = memoryMB
	snapshot.TotalDisk = diskMB
	reserved, err := s.activeReservedCapacity(ctx, querier, nodeID)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	snapshot.AvailableCPU = availableResource(snapshot.TotalCPU, snapshot.AllocatedCPU+reserved.CPU)
	snapshot.AvailableMemory = availableResource(snapshot.TotalMemory, snapshot.AllocatedMemory+reserved.Memory)
	snapshot.AvailableDisk = availableResource(snapshot.TotalDisk, snapshot.AllocatedDisk+reserved.Disk)
	return snapshot, nil
}

func (s *Store) RegionCapacitySnapshots(ctx context.Context, regionID string) ([]NodeCapacitySnapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT n.id::text, n.region_id::text,
		       COALESCE(n.cpu_threads, 0),
		       COALESCE(NULLIF(n.node_memory_mb, 0), n.memory_mb, 0),
		       COALESCE(NULLIF(n.node_disk_mb, 0), n.disk_mb, 0),
		       COALESCE((SELECT SUM(s.cpu_shares) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.cpu) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.memory_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.memory_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.disk_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.disk_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT COUNT(s.id) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT COUNT(i.id) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       now()
		FROM nodes n
		WHERE n.region_id = $1
		ORDER BY n.name
	`, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := []NodeCapacitySnapshot{}
	for rows.Next() {
		var snapshot NodeCapacitySnapshot
		var regionID sql.NullString
		var cpuThreads, memoryMB, diskMB int
		if err := rows.Scan(
			&snapshot.NodeID,
			&regionID,
			&cpuThreads,
			&memoryMB,
			&diskMB,
			&snapshot.AllocatedCPU,
			&snapshot.AllocatedMemory,
			&snapshot.AllocatedDisk,
			&snapshot.ServerCount,
			&snapshot.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if regionID.Valid {
			snapshot.RegionID = regionID.String
		}
		snapshot.TotalCPU = cpuThreads * 1024
		snapshot.TotalMemory = memoryMB
		snapshot.TotalDisk = diskMB
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for i := range snapshots {
		reserved, err := s.activeReservedCapacity(ctx, s.db, snapshots[i].NodeID)
		if err != nil {
			return nil, err
		}
		snapshots[i].AvailableCPU = availableResource(snapshots[i].TotalCPU, snapshots[i].AllocatedCPU+reserved.CPU)
		snapshots[i].AvailableMemory = availableResource(snapshots[i].TotalMemory, snapshots[i].AllocatedMemory+reserved.Memory)
		snapshots[i].AvailableDisk = availableResource(snapshots[i].TotalDisk, snapshots[i].AllocatedDisk+reserved.Disk)
	}
	return snapshots, nil
}

func (s *Store) FindAvailableAllocation(ctx context.Context, nodeID string) (Allocation, error) {
	var allocation Allocation
	var alias sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT a.id::text, n.name, host(a.ip), a.port, a.container_port, a.protocol, a.alias, COALESCE(a.notes, '')
		FROM allocations a
		JOIN nodes n ON n.id = a.node_id
		WHERE a.node_id = $1 AND a.server_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM migration_allocation_reservations mar WHERE mar.allocation_id = a.id)
		ORDER BY a.port
		LIMIT 1
	`, nodeID).Scan(&allocation.ID, &allocation.Node, &allocation.IP, &allocation.Port, &allocation.ContainerPort, &allocation.Protocol, &alias, &allocation.Notes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Allocation{}, errors.New("no available allocation on selected node")
		}
		return Allocation{}, err
	}
	if alias.Valid && alias.String != "" {
		allocation.Alias = &alias.String
	}
	return allocation, nil
}

func (s *Store) lockedNodeCapacitySnapshotTx(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, nodeID string) (NodeCapacitySnapshot, error) {
	var snapshot NodeCapacitySnapshot
	var regionID sql.NullString
	var cpuThreads, memoryMB, diskMB int
	err := querier.QueryRow(ctx, `
		SELECT n.id::text, n.region_id::text,
		       COALESCE(n.cpu_threads, 0),
		       COALESCE(NULLIF(n.node_memory_mb, 0), n.memory_mb, 0),
		       COALESCE(NULLIF(n.node_disk_mb, 0), n.disk_mb, 0),
		       COALESCE((SELECT SUM(s.cpu_shares) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.cpu) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.memory_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.memory_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT SUM(s.disk_mb) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT SUM(i.disk_mb) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       COALESCE((SELECT COUNT(s.id) FILTER (WHERE s.status <> 'deleted') FROM servers s WHERE s.node_id = n.id), 0)::int
		         + COALESCE((SELECT COUNT(i.id) FILTER (WHERE i.status NOT IN ('removing', 'failed')) FROM instances i WHERE i.node_id = n.id), 0)::int,
		       now()
		FROM nodes n
		WHERE n.id = $1
		FOR UPDATE
	`, nodeID).Scan(
		&snapshot.NodeID,
		&regionID,
		&cpuThreads,
		&memoryMB,
		&diskMB,
		&snapshot.AllocatedCPU,
		&snapshot.AllocatedMemory,
		&snapshot.AllocatedDisk,
		&snapshot.ServerCount,
		&snapshot.UpdatedAt,
	)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	if regionID.Valid {
		snapshot.RegionID = regionID.String
	}
	snapshot.TotalCPU = cpuThreads * 1024
	snapshot.TotalMemory = memoryMB
	snapshot.TotalDisk = diskMB
	reserved, err := s.activeReservedCapacity(ctx, querier, nodeID)
	if err != nil {
		return NodeCapacitySnapshot{}, err
	}
	snapshot.AvailableCPU = availableResource(snapshot.TotalCPU, snapshot.AllocatedCPU+reserved.CPU)
	snapshot.AvailableMemory = availableResource(snapshot.TotalMemory, snapshot.AllocatedMemory+reserved.Memory)
	snapshot.AvailableDisk = availableResource(snapshot.TotalDisk, snapshot.AllocatedDisk+reserved.Disk)
	return snapshot, nil
}

func availableResource(total, allocated int) int {
	if total <= 0 {
		return 0
	}
	available := total - allocated
	if available < 0 {
		return 0
	}
	return available
}
