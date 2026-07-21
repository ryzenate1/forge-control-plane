package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type NodeMetric struct {
	ID              string    `json:"id"`
	NodeID          string    `json:"nodeId"`
	CPUPercent      float64   `json:"cpuPercent"`
	MemoryPercent   float64   `json:"memoryPercent"`
	DiskPercent     float64   `json:"diskPercent"`
	MemoryUsedMB    int64     `json:"memoryUsedMb"`
	MemoryTotalMB   int64     `json:"memoryTotalMb"`
	DiskUsedMB      int64     `json:"diskUsedMb"`
	DiskTotalMB     int64     `json:"diskTotalMb"`
	CPULoad1m       float64   `json:"cpuLoad1m"`
	CPULoad5m       float64   `json:"cpuLoad5m"`
	CPULoad15m      float64   `json:"cpuLoad15m"`
	NetworkRxBytes  int64     `json:"networkRxBytes"`
	NetworkTxBytes  int64     `json:"networkTxBytes"`
	ContainerRunning int      `json:"containerRunning"`
	ContainerTotal  int       `json:"containerTotal"`
	ObservedAt      time.Time `json:"observedAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

type CreateNodeMetricRequest struct {
	NodeID           string
	CPUPercent       float64
	MemoryPercent    float64
	DiskPercent      float64
	MemoryUsedMB     int64
	MemoryTotalMB    int64
	DiskUsedMB       int64
	DiskTotalMB      int64
	CPULoad1m        float64
	CPULoad5m        float64
	CPULoad15m       float64
	NetworkRxBytes   int64
	NetworkTxBytes   int64
	ContainerRunning int
	ContainerTotal   int
	ObservedAt       time.Time
}

type WorkloadMetric struct {
	ID            string    `json:"id"`
	ServerID      string    `json:"serverId"`
	NodeID        string    `json:"nodeId"`
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	CPUPercent    float64   `json:"cpuPercent"`
	MemoryPercent float64   `json:"memoryPercent"`
	MemoryUsedMB  int64     `json:"memoryUsedMb"`
	MemoryLimitMB int64     `json:"memoryLimitMb"`
	DiskReadBytes int64     `json:"diskReadBytes"`
	DiskWriteBytes int64    `json:"diskWriteBytes"`
	NetworkRxBytes int64    `json:"networkRxBytes"`
	NetworkTxBytes int64    `json:"networkTxBytes"`
	PIDs           int       `json:"pids"`
	ObservedAt     time.Time `json:"observedAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateWorkloadMetricRequest struct {
	ServerID      string
	NodeID        string
	ContainerID   string
	ContainerName string
	CPUPercent    float64
	MemoryPercent float64
	MemoryUsedMB  int64
	MemoryLimitMB int64
	DiskReadBytes int64
	DiskWriteBytes int64
	NetworkRxBytes int64
	NetworkTxBytes int64
	PIDs           int
	ObservedAt     time.Time
}

func (s *Store) CreateNodeMetric(ctx context.Context, req CreateNodeMetricRequest) (NodeMetric, error) {
	id := uuid.NewString()
	if req.ObservedAt.IsZero() {
		req.ObservedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO node_metrics (
			id, node_id, cpu_percent, memory_percent, disk_percent,
			memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
			cpu_load_1m, cpu_load_5m, cpu_load_15m,
			network_rx_bytes, network_tx_bytes,
			container_running, container_total, observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
	`, id, req.NodeID, req.CPUPercent, req.MemoryPercent, req.DiskPercent,
		req.MemoryUsedMB, req.MemoryTotalMB, req.DiskUsedMB, req.DiskTotalMB,
		req.CPULoad1m, req.CPULoad5m, req.CPULoad15m,
		req.NetworkRxBytes, req.NetworkTxBytes,
		req.ContainerRunning, req.ContainerTotal, req.ObservedAt)
	if err != nil {
		return NodeMetric{}, err
	}
	return s.GetNodeMetric(ctx, id)
}

func (s *Store) GetNodeMetric(ctx context.Context, id string) (NodeMetric, error) {
	var m NodeMetric
	err := s.db.QueryRow(ctx, `
		SELECT id::text, node_id::text, cpu_percent, memory_percent, disk_percent,
			memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
			cpu_load_1m, cpu_load_5m, cpu_load_15m,
			network_rx_bytes, network_tx_bytes,
			container_running, container_total, observed_at, created_at
		FROM node_metrics WHERE id = $1
	`, id).Scan(&m.ID, &m.NodeID, &m.CPUPercent, &m.MemoryPercent, &m.DiskPercent,
		&m.MemoryUsedMB, &m.MemoryTotalMB, &m.DiskUsedMB, &m.DiskTotalMB,
		&m.CPULoad1m, &m.CPULoad5m, &m.CPULoad15m,
		&m.NetworkRxBytes, &m.NetworkTxBytes,
		&m.ContainerRunning, &m.ContainerTotal, &m.ObservedAt, &m.CreatedAt)
	return m, err
}

func (s *Store) ListNodeMetrics(ctx context.Context, nodeID string, limit int, since *time.Time) ([]NodeMetric, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args := []any{nodeID, limit}
	query := `SELECT id::text, node_id::text, cpu_percent, memory_percent, disk_percent,
		memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
		cpu_load_1m, cpu_load_5m, cpu_load_15m,
		network_rx_bytes, network_tx_bytes,
		container_running, container_total, observed_at, created_at
		FROM node_metrics WHERE node_id = $1`
	if since != nil {
		query += ` AND observed_at >= $3`
		args = append(args, *since)
	}
	query += ` ORDER BY observed_at DESC LIMIT $2`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NodeMetric{}
	for rows.Next() {
		var m NodeMetric
		if err := rows.Scan(&m.ID, &m.NodeID, &m.CPUPercent, &m.MemoryPercent, &m.DiskPercent,
			&m.MemoryUsedMB, &m.MemoryTotalMB, &m.DiskUsedMB, &m.DiskTotalMB,
			&m.CPULoad1m, &m.CPULoad5m, &m.CPULoad15m,
			&m.NetworkRxBytes, &m.NetworkTxBytes,
			&m.ContainerRunning, &m.ContainerTotal, &m.ObservedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *Store) ListAllNodeMetrics(ctx context.Context, limit int) ([]NodeMetric, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, node_id::text, cpu_percent, memory_percent, disk_percent,
			memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
			cpu_load_1m, cpu_load_5m, cpu_load_15m,
			network_rx_bytes, network_tx_bytes,
			container_running, container_total, observed_at, created_at
		FROM node_metrics
		ORDER BY observed_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NodeMetric{}
	for rows.Next() {
		var m NodeMetric
		if err := rows.Scan(&m.ID, &m.NodeID, &m.CPUPercent, &m.MemoryPercent, &m.DiskPercent,
			&m.MemoryUsedMB, &m.MemoryTotalMB, &m.DiskUsedMB, &m.DiskTotalMB,
			&m.CPULoad1m, &m.CPULoad5m, &m.CPULoad15m,
			&m.NetworkRxBytes, &m.NetworkTxBytes,
			&m.ContainerRunning, &m.ContainerTotal, &m.ObservedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *Store) ListNodeMetricsLatest(ctx context.Context) ([]NodeMetric, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (node_id) id::text, node_id::text, cpu_percent, memory_percent, disk_percent,
			memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb,
			cpu_load_1m, cpu_load_5m, cpu_load_15m,
			network_rx_bytes, network_tx_bytes,
			container_running, container_total, observed_at, created_at
		FROM node_metrics
		ORDER BY node_id, observed_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NodeMetric{}
	for rows.Next() {
		var m NodeMetric
		if err := rows.Scan(&m.ID, &m.NodeID, &m.CPUPercent, &m.MemoryPercent, &m.DiskPercent,
			&m.MemoryUsedMB, &m.MemoryTotalMB, &m.DiskUsedMB, &m.DiskTotalMB,
			&m.CPULoad1m, &m.CPULoad5m, &m.CPULoad15m,
			&m.NetworkRxBytes, &m.NetworkTxBytes,
			&m.ContainerRunning, &m.ContainerTotal, &m.ObservedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *Store) CreateWorkloadMetric(ctx context.Context, req CreateWorkloadMetricRequest) (WorkloadMetric, error) {
	id := uuid.NewString()
	if req.ObservedAt.IsZero() {
		req.ObservedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO workload_metrics (
			id, server_id, node_id, container_id, container_name,
			cpu_percent, memory_percent, memory_used_mb, memory_limit_mb,
			disk_read_bytes, disk_write_bytes,
			network_rx_bytes, network_tx_bytes, pids, observed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, id, req.ServerID, req.NodeID, req.ContainerID, req.ContainerName,
		req.CPUPercent, req.MemoryPercent, req.MemoryUsedMB, req.MemoryLimitMB,
		req.DiskReadBytes, req.DiskWriteBytes,
		req.NetworkRxBytes, req.NetworkTxBytes, req.PIDs, req.ObservedAt)
	if err != nil {
		return WorkloadMetric{}, err
	}
	var m WorkloadMetric
	err = s.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(server_id::text,''), node_id::text, container_id, container_name,
			cpu_percent, memory_percent, memory_used_mb, memory_limit_mb,
			disk_read_bytes, disk_write_bytes,
			network_rx_bytes, network_tx_bytes, pids, observed_at, created_at
		FROM workload_metrics WHERE id = $1
	`, id).Scan(&m.ID, &m.ServerID, &m.NodeID, &m.ContainerID, &m.ContainerName,
		&m.CPUPercent, &m.MemoryPercent, &m.MemoryUsedMB, &m.MemoryLimitMB,
		&m.DiskReadBytes, &m.DiskWriteBytes,
		&m.NetworkRxBytes, &m.NetworkTxBytes, &m.PIDs, &m.ObservedAt, &m.CreatedAt)
	return m, err
}

func (s *Store) ListWorkloadMetrics(ctx context.Context, serverID string, limit int) ([]WorkloadMetric, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, COALESCE(server_id::text,''), node_id::text, container_id, container_name,
			cpu_percent, memory_percent, memory_used_mb, memory_limit_mb,
			disk_read_bytes, disk_write_bytes,
			network_rx_bytes, network_tx_bytes, pids, observed_at, created_at
		FROM workload_metrics
		WHERE server_id = $1
		ORDER BY observed_at DESC LIMIT $2
	`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkloadMetric{}
	for rows.Next() {
		var m WorkloadMetric
		if err := rows.Scan(&m.ID, &m.ServerID, &m.NodeID, &m.ContainerID, &m.ContainerName,
			&m.CPUPercent, &m.MemoryPercent, &m.MemoryUsedMB, &m.MemoryLimitMB,
			&m.DiskReadBytes, &m.DiskWriteBytes,
			&m.NetworkRxBytes, &m.NetworkTxBytes, &m.PIDs, &m.ObservedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *Store) PruneNodeMetrics(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM node_metrics WHERE observed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) PruneWorkloadMetrics(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM workload_metrics WHERE observed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}
