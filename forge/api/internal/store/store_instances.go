package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReplicaApplication struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Replicas        int       `json:"replicas"`
	CPU             int       `json:"cpu"`
	MemoryMB        int       `json:"memoryMb"`
	DiskMB          int       `json:"diskMb"`
	RuntimeProvider string    `json:"runtimeProvider"`
	Image           string    `json:"image"`
	Status          string    `json:"status"`
	Generation      int       `json:"generation"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type CreateReplicaAppRequest struct {
	Name            string
	Replicas        int
	CPU             int
	MemoryMB        int
	DiskMB          int
	RuntimeProvider string
	Image           string
}

type Instance struct {
	ID              string     `json:"id"`
	AppID           string     `json:"appId"`
	Idx             int        `json:"idx"`
	NodeID          string     `json:"nodeId"`
	Status          string     `json:"status"`
	CPU             int        `json:"cpu"`
	MemoryMB        int        `json:"memoryMb"`
	DiskMB          int        `json:"diskMb"`
	AllocationID    *string    `json:"allocationId,omitempty"`
	PlacementID     string     `json:"placementId"`
	ReservationID   *string    `json:"reservationId,omitempty"`
	RuntimeProvider string     `json:"runtimeProvider"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type PlacementDecision struct {
	ID              string    `json:"id"`
	InstanceID      string    `json:"instanceId"`
	NodeID          string    `json:"nodeId"`
	AppID           string    `json:"appId"`
	Idx             int       `json:"idx"`
	Score           float64   `json:"score"`
	Accepted        bool      `json:"accepted"`
	Reasons         []string  `json:"reasons"`
	RuntimeProvider string    `json:"runtimeProvider"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (s *Store) CreateReplicaApp(ctx context.Context, req CreateReplicaAppRequest) (ReplicaApplication, error) {
	id := uuid.NewString()
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	if req.CPU <= 0 {
		req.CPU = 1024
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = 2048
	}
	if req.DiskMB <= 0 {
		req.DiskMB = 10240
	}
	if req.RuntimeProvider == "" {
		req.RuntimeProvider = "docker"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO replica_applications (id, name, replicas, cpu, memory_mb, disk_mb, runtime_provider, image)
		VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE(NULLIF($8, ''), 'nginx:alpine'))
	`, id, req.Name, req.Replicas, req.CPU, req.MemoryMB, req.DiskMB, req.RuntimeProvider, req.Image)
	if err != nil {
		return ReplicaApplication{}, err
	}
	return s.GetReplicaApp(ctx, id)
}

func (s *Store) GetReplicaApp(ctx context.Context, id string) (ReplicaApplication, error) {
	var app ReplicaApplication
		err := s.db.QueryRow(ctx, `
		SELECT id::text, name, replicas, cpu, memory_mb, disk_mb, runtime_provider, COALESCE(image, 'nginx:alpine'), COALESCE(status, 'pending'), COALESCE(generation, 0), created_at, updated_at
		FROM replica_applications WHERE id = $1
	`, id).Scan(&app.ID, &app.Name, &app.Replicas, &app.CPU, &app.MemoryMB, &app.DiskMB, &app.RuntimeProvider, &app.Image, &app.Status, &app.Generation, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return ReplicaApplication{}, err
	}
	return app, nil
}

func (s *Store) ListReplicaApps(ctx context.Context) ([]ReplicaApplication, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, replicas, cpu, memory_mb, disk_mb, runtime_provider, COALESCE(image, 'nginx:alpine'), COALESCE(status, 'pending'), COALESCE(generation, 0), created_at, updated_at
		FROM replica_applications ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []ReplicaApplication
	for rows.Next() {
		var app ReplicaApplication
		if err := rows.Scan(&app.ID, &app.Name, &app.Replicas, &app.CPU, &app.MemoryMB, &app.DiskMB, &app.RuntimeProvider, &app.Image, &app.Status, &app.Generation, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *Store) UpdateReplicaAppReplicas(ctx context.Context, appID string, replicas int) (ReplicaApplication, error) {
	if replicas < 0 {
		return ReplicaApplication{}, errors.New("replicas must be non-negative")
	}
	_, err := s.db.Exec(ctx, `UPDATE replica_applications SET replicas = $1, updated_at = now() WHERE id = $2`, replicas, appID)
	if err != nil {
		return ReplicaApplication{}, err
	}
	return s.GetReplicaApp(ctx, appID)
}

func (s *Store) UpdateReplicaAppStatus(ctx context.Context, appID, status string) (ReplicaApplication, error) {
	_, err := s.db.Exec(ctx, `UPDATE replica_applications SET status = $1, updated_at = now() WHERE id = $2`, status, appID)
	if err != nil {
		return ReplicaApplication{}, err
	}
	return s.GetReplicaApp(ctx, appID)
}

func (s *Store) IncrementReplicaAppGeneration(ctx context.Context, appID string) (int, error) {
	var gen int
	err := s.db.QueryRow(ctx, `
		UPDATE replica_applications SET generation = generation + 1, updated_at = now()
		WHERE id = $1
		RETURNING generation
	`, appID).Scan(&gen)
	if err != nil {
		return 0, err
	}
	return gen, nil
}

func (s *Store) DeleteReplicaApp(ctx context.Context, appID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM replica_applications WHERE id = $1`, appID)
	return err
}

func (s *Store) CreateInstance(ctx context.Context, tx pgx.Tx, appID, nodeID string, idx int, cpu, memoryMB, diskMB int, runtimeProvider string) (Instance, error) {
	id := uuid.NewString()
	placementID := uuid.NewString()
	if runtimeProvider == "" {
		runtimeProvider = "docker"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO instances (id, app_id, idx, node_id, status, cpu, memory_mb, disk_mb, placement_id, runtime_provider)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9)
	`, id, appID, idx, nodeID, cpu, memoryMB, diskMB, placementID, runtimeProvider)
	if err != nil {
		return Instance{}, err
	}
	return s.GetInstance(ctx, id)
}

func (s *Store) GetInstance(ctx context.Context, id string) (Instance, error) {
	var inst Instance
	var allocID, resID sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT id::text, app_id::text, idx, node_id::text, status, cpu, memory_mb, disk_mb,
		       allocation_id::text, placement_id::text, reservation_id::text, runtime_provider, created_at, updated_at
		FROM instances WHERE id = $1
	`, id).Scan(&inst.ID, &inst.AppID, &inst.Idx, &inst.NodeID, &inst.Status, &inst.CPU, &inst.MemoryMB, &inst.DiskMB,
		&allocID, &inst.PlacementID, &resID, &inst.RuntimeProvider, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return Instance{}, err
	}
	if allocID.Valid {
		inst.AllocationID = &allocID.String
	}
	if resID.Valid {
		inst.ReservationID = &resID.String
	}
	return inst, nil
}

func (s *Store) ListInstancesByApp(ctx context.Context, appID string) ([]Instance, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, app_id::text, idx, node_id::text, status, cpu, memory_mb, disk_mb,
		       allocation_id::text, placement_id::text, reservation_id::text, runtime_provider, created_at, updated_at
		FROM instances WHERE app_id = $1 ORDER BY idx
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func (s *Store) ListInstancesByNode(ctx context.Context, nodeID string) ([]Instance, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, app_id::text, idx, node_id::text, status, cpu, memory_mb, disk_mb,
		       allocation_id::text, placement_id::text, reservation_id::text, runtime_provider, created_at, updated_at
		FROM instances WHERE node_id = $1 ORDER BY created_at
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func scanInstances(rows pgx.Rows) ([]Instance, error) {
	var insts []Instance
	for rows.Next() {
		var inst Instance
		var allocID, resID sql.NullString
		if err := rows.Scan(&inst.ID, &inst.AppID, &inst.Idx, &inst.NodeID, &inst.Status, &inst.CPU, &inst.MemoryMB, &inst.DiskMB,
			&allocID, &inst.PlacementID, &resID, &inst.RuntimeProvider, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		if allocID.Valid {
			inst.AllocationID = &allocID.String
		}
		if resID.Valid {
			inst.ReservationID = &resID.String
		}
		insts = append(insts, inst)
	}
	return insts, rows.Err()
}

func (s *Store) UpdateInstanceStatus(ctx context.Context, id, status string) (Instance, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE instances SET status = $1, updated_at = now() WHERE id = $2
	`, status, id)
	if err != nil {
		return Instance{}, err
	}
	return s.GetInstance(ctx, id)
}

func (s *Store) UpdateInstanceNode(ctx context.Context, id, nodeID string) (Instance, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE instances SET node_id = $1, updated_at = now() WHERE id = $2
	`, nodeID, id)
	if err != nil {
		return Instance{}, err
	}
	return s.GetInstance(ctx, id)
}

func (s *Store) AssignInstanceAllocation(ctx context.Context, id, allocationID string) (Instance, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE instances SET allocation_id = $1, updated_at = now() WHERE id = $2
	`, allocationID, id)
	if err != nil {
		return Instance{}, err
	}
	return s.GetInstance(ctx, id)
}

func (s *Store) AssignInstanceReservation(ctx context.Context, id, reservationID string) (Instance, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE instances SET reservation_id = $1, updated_at = now() WHERE id = $2
	`, reservationID, id)
	if err != nil {
		return Instance{}, err
	}
	return s.GetInstance(ctx, id)
}

func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM instances WHERE id = $1`, id)
	return err
}

func (s *Store) CreatePlacementDecision(ctx context.Context, instanceID, nodeID, appID string, idx int, score float64, accepted bool, reasons []string, runtimeProvider string) (PlacementDecision, error) {
	id := uuid.NewString()
	if reasons == nil {
		reasons = []string{}
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO placement_decisions (id, instance_id, node_id, app_id, idx, score, accepted, reasons, runtime_provider)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, instanceID, nodeID, appID, idx, score, accepted, reasons, runtimeProvider)
	if err != nil {
		return PlacementDecision{}, err
	}
	return s.GetPlacementDecision(ctx, id)
}

func (s *Store) GetPlacementDecision(ctx context.Context, id string) (PlacementDecision, error) {
	var pd PlacementDecision
	err := s.db.QueryRow(ctx, `
		SELECT id::text, instance_id::text, node_id::text, app_id::text, idx, score, accepted, reasons, runtime_provider, created_at
		FROM placement_decisions WHERE id = $1
	`, id).Scan(&pd.ID, &pd.InstanceID, &pd.NodeID, &pd.AppID, &pd.Idx, &pd.Score, &pd.Accepted, &pd.Reasons, &pd.RuntimeProvider, &pd.CreatedAt)
	if err != nil {
		return PlacementDecision{}, err
	}
	return pd, nil
}

func (s *Store) ListPlacementDecisionsByApp(ctx context.Context, appID string) ([]PlacementDecision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, instance_id::text, node_id::text, app_id::text, idx, score, accepted, reasons, runtime_provider, created_at
		FROM placement_decisions WHERE app_id = $1 ORDER BY created_at DESC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlacementDecisions(rows)
}

func (s *Store) ListLatestPlacementPerInstance(ctx context.Context, appID string) ([]PlacementDecision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (pd.instance_id) pd.id::text, pd.instance_id::text, pd.node_id::text, pd.app_id::text,
		       pd.idx, pd.score, pd.accepted, pd.reasons, pd.runtime_provider, pd.created_at
		FROM placement_decisions pd
		WHERE pd.app_id = $1
		ORDER BY pd.instance_id, pd.created_at DESC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlacementDecisions(rows)
}

func scanPlacementDecisions(rows pgx.Rows) ([]PlacementDecision, error) {
	var decisions []PlacementDecision
	for rows.Next() {
		var pd PlacementDecision
		if err := rows.Scan(&pd.ID, &pd.InstanceID, &pd.NodeID, &pd.AppID, &pd.Idx, &pd.Score, &pd.Accepted, &pd.Reasons, &pd.RuntimeProvider, &pd.CreatedAt); err != nil {
			return nil, err
		}
		decisions = append(decisions, pd)
	}
	return decisions, rows.Err()
}

func (s *Store) ListRunningInstancesOnNode(ctx context.Context, nodeID string) ([]Instance, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, app_id::text, idx, node_id::text, status, cpu, memory_mb, disk_mb,
		       allocation_id::text, placement_id::text, reservation_id::text, runtime_provider, created_at, updated_at
		FROM instances WHERE node_id = $1 AND status NOT IN ('removing', 'failed')
		ORDER BY created_at
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func (s *Store) InstanceCapacityOnNode(ctx context.Context, nodeID string) (cpu, memory, disk int, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(cpu), 0)::int, COALESCE(SUM(memory_mb), 0)::int, COALESCE(SUM(disk_mb), 0)::int
		FROM instances WHERE node_id = $1 AND status NOT IN ('removing', 'failed')
	`, nodeID).Scan(&cpu, &memory, &disk)
	return
}
