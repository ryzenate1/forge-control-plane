package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RestoreJob struct {
	ID               string     `json:"id"`
	ArtifactID       *string    `json:"artifactId,omitempty"`
	RestoreType      string     `json:"restoreType"`
	TargetServerID   *string    `json:"targetServerId,omitempty"`
	TargetAppID      *string    `json:"targetAppId,omitempty"`
	TargetDatabaseID *string    `json:"targetDatabaseId,omitempty"`
	TargetVolumeID   *string    `json:"targetVolumeId,omitempty"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	NodeID           *string    `json:"nodeId,omitempty"`
	BeaconTaskID     *string    `json:"beaconTaskId,omitempty"`
	TriggeredBy      string     `json:"triggeredBy"`
	TriggeredByUserID *string   `json:"triggeredByUserId,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	Data             []byte     `json:"data"`
}

func (s *Store) CreateRestoreJob(ctx context.Context, r *RestoreJob) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now()
	r.CreatedAt = now
	r.UpdatedAt = now
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_restores (id, artifact_id, restore_type, target_server_id, target_app_id,
			target_database_id, target_volume_id, name, status, node_id, beacon_task_id,
			triggered_by, triggered_by_user_id, created_at, updated_at, data)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, r.ID, r.ArtifactID, r.RestoreType, r.TargetServerID, r.TargetAppID,
		r.TargetDatabaseID, r.TargetVolumeID, r.Name, r.Status, r.NodeID, r.BeaconTaskID,
		r.TriggeredBy, r.TriggeredByUserID, r.CreatedAt, r.UpdatedAt, r.Data)
	return err
}

func (s *Store) GetRestoreJob(ctx context.Context, id string) (RestoreJob, error) {
	var r RestoreJob
	err := s.db.QueryRow(ctx, `
		SELECT id, artifact_id, restore_type, target_server_id, target_app_id,
			target_database_id, target_volume_id, name, status, node_id, beacon_task_id,
			triggered_by, triggered_by_user_id, created_at, updated_at, data
		FROM backup_restores WHERE id = $1
	`, id).Scan(&r.ID, &r.ArtifactID, &r.RestoreType, &r.TargetServerID, &r.TargetAppID,
		&r.TargetDatabaseID, &r.TargetVolumeID, &r.Name, &r.Status, &r.NodeID, &r.BeaconTaskID,
		&r.TriggeredBy, &r.TriggeredByUserID, &r.CreatedAt, &r.UpdatedAt, &r.Data)
	return r, err
}

func (s *Store) ListRestoreJobs(ctx context.Context, status, targetServerID, targetAppID, targetDatabaseID, targetVolumeID, nodeID *string, page, perPage int) ([]RestoreJob, int, error) {
	where := []string{"1=1"}
	args := []any{}
	argN := 1

	if status != nil {
		where = append(where, fmt.Sprintf("status = $%d", argN))
		args = append(args, *status)
		argN++
	}
	if targetServerID != nil {
		where = append(where, fmt.Sprintf("target_server_id = $%d", argN))
		args = append(args, *targetServerID)
		argN++
	}
	if targetAppID != nil {
		where = append(where, fmt.Sprintf("target_app_id = $%d", argN))
		args = append(args, *targetAppID)
		argN++
	}
	if targetDatabaseID != nil {
		where = append(where, fmt.Sprintf("target_database_id = $%d", argN))
		args = append(args, *targetDatabaseID)
		argN++
	}
	if targetVolumeID != nil {
		where = append(where, fmt.Sprintf("target_volume_id = $%d", argN))
		args = append(args, *targetVolumeID)
		argN++
	}
	if nodeID != nil {
		where = append(where, fmt.Sprintf("node_id = $%d", argN))
		args = append(args, *nodeID)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow(ctx, "SELECT count(*) FROM backup_restores WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	rows, err := s.db.Query(ctx, `
		SELECT id, artifact_id, restore_type, target_server_id, target_app_id,
			target_database_id, target_volume_id, name, status, node_id, beacon_task_id,
			triggered_by, triggered_by_user_id, created_at, updated_at, data
		FROM backup_restores WHERE `+whereClause+`
		ORDER BY created_at DESC
		LIMIT $`+fmt.Sprintf("%d", argN)+` OFFSET $`+fmt.Sprintf("%d", argN+1),
		append(args, perPage, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []RestoreJob
	for rows.Next() {
		var r RestoreJob
		if err := rows.Scan(&r.ID, &r.ArtifactID, &r.RestoreType, &r.TargetServerID, &r.TargetAppID,
			&r.TargetDatabaseID, &r.TargetVolumeID, &r.Name, &r.Status, &r.NodeID, &r.BeaconTaskID,
			&r.TriggeredBy, &r.TriggeredByUserID, &r.CreatedAt, &r.UpdatedAt, &r.Data); err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, r)
	}
	return jobs, total, rows.Err()
}

func (s *Store) UpdateRestoreJob(ctx context.Context, r *RestoreJob) error {
	r.UpdatedAt = time.Now()
	_, err := s.db.Exec(ctx, `
		UPDATE backup_restores SET
			artifact_id=$2, restore_type=$3, target_server_id=$4, target_app_id=$5,
			target_database_id=$6, target_volume_id=$7, name=$8, status=$9,
			node_id=$10, beacon_task_id=$11, triggered_by=$12, triggered_by_user_id=$13,
			updated_at=$14, data=$15
		WHERE id=$1
	`, r.ID, r.ArtifactID, r.RestoreType, r.TargetServerID, r.TargetAppID,
		r.TargetDatabaseID, r.TargetVolumeID, r.Name, r.Status, r.NodeID, r.BeaconTaskID,
		r.TriggeredBy, r.TriggeredByUserID, r.UpdatedAt, r.Data)
	return err
}

func (s *Store) DeleteRestoreJob(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM backup_restores WHERE id = $1`, id)
	return err
}
