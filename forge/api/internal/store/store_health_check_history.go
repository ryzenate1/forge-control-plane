package store

import (
	"context"
	"time"
)

type HealthCheckHistoryRow struct {
	ID           int64     `json:"id"`
	TargetID     string    `json:"targetId"`
	GroupID      string    `json:"groupId"`
	ServerID     string    `json:"serverId"`
	CheckType    string    `json:"checkType"`
	Status       string    `json:"status"`
	LatencyMs    int       `json:"latencyMs"`
	StatusCode   int       `json:"statusCode"`
	ErrorMessage string    `json:"errorMessage"`
	CheckedAt    time.Time `json:"checkedAt"`
}

func (s *Store) InsertHealthCheckHistory(ctx context.Context, row HealthCheckHistoryRow) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO health_check_history (target_id, group_id, server_id, check_type, status, latency_ms, status_code, error_message, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, row.TargetID, row.GroupID, row.ServerID, row.CheckType, row.Status, row.LatencyMs, row.StatusCode, row.ErrorMessage, row.CheckedAt)
	return err
}

func (s *Store) ListHealthCheckHistory(ctx context.Context, targetID string, limit int) ([]HealthCheckHistoryRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, target_id, group_id, server_id, check_type, status, latency_ms, status_code, error_message, checked_at
		FROM health_check_history
		WHERE target_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]HealthCheckHistoryRow, 0, limit)
	for rows.Next() {
		var r HealthCheckHistoryRow
		if err := rows.Scan(&r.ID, &r.TargetID, &r.GroupID, &r.ServerID, &r.CheckType, &r.Status, &r.LatencyMs, &r.StatusCode, &r.ErrorMessage, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) ListHealthCheckHistoryByServer(ctx context.Context, serverID string, since time.Time) ([]HealthCheckHistoryRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, target_id, group_id, server_id, check_type, status, latency_ms, status_code, error_message, checked_at
		FROM health_check_history
		WHERE server_id = $1 AND checked_at >= $2
		ORDER BY checked_at DESC
	`, serverID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]HealthCheckHistoryRow, 0)
	for rows.Next() {
		var r HealthCheckHistoryRow
		if err := rows.Scan(&r.ID, &r.TargetID, &r.GroupID, &r.ServerID, &r.CheckType, &r.Status, &r.LatencyMs, &r.StatusCode, &r.ErrorMessage, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) GetTargetHealthSummary(ctx context.Context, targetID string) (consecutiveFailures int, consecutiveSuccesses int, err error) {
	rows, err := s.db.Query(ctx, `
		SELECT status FROM health_check_history
		WHERE target_id = $1
		ORDER BY checked_at DESC
		LIMIT 100
	`, targetID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return 0, 0, err
		}
		if status == "unhealthy" || status == "suspected" {
			if consecutiveSuccesses == 0 {
				consecutiveFailures++
			}
		} else {
			if consecutiveFailures == 0 {
				consecutiveSuccesses++
			}
		}
	}
	return consecutiveFailures, consecutiveSuccesses, rows.Err()
}

func (s *Store) PruneHealthCheckHistory(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM health_check_history WHERE checked_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
