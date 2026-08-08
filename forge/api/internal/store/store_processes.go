package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ProcessType struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"serverId"`
	ProcessType string    `json:"processType"`
	Command     string    `json:"command"`
	Quantity    int       `json:"quantity"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProcessScalingEvent struct {
	ID           string    `json:"id"`
	ServerID     string    `json:"serverId"`
	ProcessType  string    `json:"processType"`
	OldQuantity  int       `json:"oldQuantity"`
	NewQuantity  int       `json:"newQuantity"`
	TriggeredBy  string    `json:"triggeredBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

type OneOffTask struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	Command     string     `json:"command"`
	Status      string     `json:"status"`
	Output      string     `json:"output"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

func (s *Store) ListProcessTypes(ctx context.Context, serverID string) ([]ProcessType, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, process_type, COALESCE(command, ''), quantity, created_at, updated_at
		FROM process_types WHERE server_id = $1 ORDER BY process_type
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ProcessType
	for rows.Next() {
		var pt ProcessType
		if err := rows.Scan(&pt.ID, &pt.ServerID, &pt.ProcessType, &pt.Command, &pt.Quantity, &pt.CreatedAt, &pt.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, pt)
	}
	return result, nil
}

func (s *Store) GetProcessType(ctx context.Context, serverID, processType string) (ProcessType, error) {
	var pt ProcessType
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, process_type, COALESCE(command, ''), quantity, created_at, updated_at
		FROM process_types WHERE server_id = $1 AND process_type = $2
	`, serverID, processType).Scan(&pt.ID, &pt.ServerID, &pt.ProcessType, &pt.Command, &pt.Quantity, &pt.CreatedAt, &pt.UpdatedAt)
	if err != nil {
		return ProcessType{}, err
	}
	return pt, nil
}

func (s *Store) UpsertProcessType(ctx context.Context, serverID, processType, command string, quantity int) (ProcessType, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO process_types (id, server_id, process_type, command, quantity)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (server_id, process_type)
		DO UPDATE SET command = EXCLUDED.command, quantity = EXCLUDED.quantity, updated_at = NOW()
	`, id, serverID, processType, command, quantity)
	if err != nil {
		return ProcessType{}, err
	}
	return s.GetProcessType(ctx, serverID, processType)
}

func (s *Store) DeleteProcessType(ctx context.Context, serverID, processType string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM process_types WHERE server_id = $1 AND process_type = $2`, serverID, processType)
	return err
}

func (s *Store) SetProcessTypeQuantity(ctx context.Context, serverID, processType string, quantity int) error {
	_, err := s.db.Exec(ctx, `UPDATE process_types SET quantity = $1, updated_at = NOW() WHERE server_id = $2 AND process_type = $3`, quantity, serverID, processType)
	return err
}

func (s *Store) CreateProcessScalingEvent(ctx context.Context, serverID, processType string, oldQuantity, newQuantity int, triggeredBy string) (ProcessScalingEvent, error) {
	id := uuid.NewString()
	var ev ProcessScalingEvent
	err := s.db.QueryRow(ctx, `
		INSERT INTO process_scaling_events (id, server_id, process_type, old_quantity, new_quantity, triggered_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, server_id::text, process_type, old_quantity, new_quantity, triggered_by, created_at
	`, id, serverID, processType, oldQuantity, newQuantity, triggeredBy).Scan(
		&ev.ID, &ev.ServerID, &ev.ProcessType, &ev.OldQuantity, &ev.NewQuantity, &ev.TriggeredBy, &ev.CreatedAt)
	if err != nil {
		return ProcessScalingEvent{}, err
	}
	return ev, nil
}

func (s *Store) GetScalingHistory(ctx context.Context, serverID string) ([]ProcessScalingEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, process_type, old_quantity, new_quantity, triggered_by, created_at
		FROM process_scaling_events WHERE server_id = $1 ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ProcessScalingEvent
	for rows.Next() {
		var ev ProcessScalingEvent
		if err := rows.Scan(&ev.ID, &ev.ServerID, &ev.ProcessType, &ev.OldQuantity, &ev.NewQuantity, &ev.TriggeredBy, &ev.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, nil
}

func (s *Store) CreateOneOffTask(ctx context.Context, serverID, command string) (OneOffTask, error) {
	id := uuid.NewString()
	var task OneOffTask
	err := s.db.QueryRow(ctx, `
		INSERT INTO one_off_tasks (id, server_id, command)
		VALUES ($1, $2, $3)
		RETURNING id::text, server_id::text, command, status, output, created_at, completed_at
	`, id, serverID, command).Scan(&task.ID, &task.ServerID, &task.Command, &task.Status, &task.Output, &task.CreatedAt, &task.CompletedAt)
	if err != nil {
		return OneOffTask{}, err
	}
	return task, nil
}

func (s *Store) UpdateOneOffTask(ctx context.Context, id, status, output string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE one_off_tasks SET status = $1, output = $2, completed_at = CASE WHEN $1 IN ('completed','failed') THEN NOW() ELSE completed_at END
		WHERE id = $3
	`, status, output, id)
	return err
}

func (s *Store) GetOneOffTask(ctx context.Context, id string) (OneOffTask, error) {
	var task OneOffTask
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, command, status, output, created_at, completed_at
		FROM one_off_tasks WHERE id = $1
	`, id).Scan(&task.ID, &task.ServerID, &task.Command, &task.Status, &task.Output, &task.CreatedAt, &task.CompletedAt)
	if err != nil {
		return OneOffTask{}, err
	}
	return task, nil
}

func (s *Store) ListOneOffTasks(ctx context.Context, serverID string) ([]OneOffTask, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, command, status, output, created_at, completed_at
		FROM one_off_tasks WHERE server_id = $1 ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OneOffTask
	for rows.Next() {
		var task OneOffTask
		if err := rows.Scan(&task.ID, &task.ServerID, &task.Command, &task.Status, &task.Output, &task.CreatedAt, &task.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, nil
}
