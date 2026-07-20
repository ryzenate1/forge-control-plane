package store

import (
	"context"
	"errors"
	"strings"

	"gamepanel/forge/internal/platform/operations"

	"github.com/google/uuid"
)

// CreateOperation persists a durable operation independently of the legacy
// queue. Queue-backed modules may then attach a job or an agent command.
func (s *Store) CreateOperation(ctx context.Context, request operations.Request) (operations.Operation, error) {
	if strings.TrimSpace(request.Kind) == "" || strings.TrimSpace(request.ResourceType) == "" || strings.TrimSpace(request.ResourceID) == "" {
		return operations.Operation{}, errors.New("operation kind, resource type, and resource id are required")
	}
	if request.DesiredGeneration < 1 {
		return operations.Operation{}, errors.New("operation desired generation must be positive")
	}
	if len(request.Input) == 0 {
		request.Input = []byte(`{}`)
	}
	value := operations.Operation{}
	err := s.db.QueryRow(ctx, `INSERT INTO operations(id,kind,resource_type,resource_id,status,idempotency_key,desired_generation,input) VALUES($1,$2,$3,$4,'queued',NULLIF($5,''),$6,$7::jsonb) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key RETURNING id::text,kind,resource_type,resource_id,status,desired_generation,observed_generation,error,created_at,updated_at`, uuid.NewString(), strings.TrimSpace(request.Kind), strings.TrimSpace(request.ResourceType), strings.TrimSpace(request.ResourceID), strings.TrimSpace(request.IdempotencyKey), request.DesiredGeneration, []byte(request.Input)).Scan(&value.ID, &value.Kind, &value.ResourceType, &value.ResourceID, &value.Status, &value.DesiredGeneration, &value.ObservedGeneration, &value.Error, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) GetOperation(ctx context.Context, id string) (operations.Operation, error) {
	value := operations.Operation{}
	err := s.db.QueryRow(ctx, `SELECT id::text,kind,resource_type,resource_id,status,desired_generation,observed_generation,error,created_at,updated_at FROM operations WHERE id=$1::uuid`, id).Scan(&value.ID, &value.Kind, &value.ResourceType, &value.ResourceID, &value.Status, &value.DesiredGeneration, &value.ObservedGeneration, &value.Error, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (s *Store) ListOperations(ctx context.Context, resourceID string, limit int) ([]operations.Operation, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id::text,kind,resource_type,resource_id,status,desired_generation,observed_generation,error,created_at,updated_at FROM operations WHERE ($1='' OR resource_id=$1) ORDER BY created_at DESC LIMIT $2`, strings.TrimSpace(resourceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []operations.Operation{}
	for rows.Next() {
		var value operations.Operation
		if err := rows.Scan(&value.ID, &value.Kind, &value.ResourceType, &value.ResourceID, &value.Status, &value.DesiredGeneration, &value.ObservedGeneration, &value.Error, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) UpdateOperationStatus(ctx context.Context, id string, status operations.Status, operationError string) error {
	if !validPlatformOperationStatus(status) {
		return errors.New("invalid operation status")
	}
	completed := status == operations.StatusSucceeded || status == operations.StatusFailed || status == operations.StatusCancelled
	result, err := s.db.Exec(ctx, `UPDATE operations SET status=$2,error=$3,started_at=CASE WHEN $2='running' THEN COALESCE(started_at,now()) ELSE started_at END,completed_at=CASE WHEN $4 THEN now() ELSE completed_at END,updated_at=now() WHERE id=$1::uuid`, id, status, strings.TrimSpace(operationError), completed)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("operation not found")
	}
	return nil
}

func validPlatformOperationStatus(status operations.Status) bool {
	switch status {
	case operations.StatusQueued, operations.StatusRunning, operations.StatusWaiting, operations.StatusRetrying, operations.StatusCancelling, operations.StatusRollingBack, operations.StatusSucceeded, operations.StatusFailed, operations.StatusCancelled:
		return true
	default:
		return false
	}
}

// StoreOperationRepository adapts Store to the platform operation port.
type StoreOperationRepository struct{ store *Store }

func NewOperationRepository(store *Store) *StoreOperationRepository {
	return &StoreOperationRepository{store: store}
}

func (r *StoreOperationRepository) Create(ctx context.Context, request operations.Request) (operations.Operation, error) {
	return r.store.CreateOperation(ctx, request)
}
func (r *StoreOperationRepository) Get(ctx context.Context, id string) (operations.Operation, error) {
	return r.store.GetOperation(ctx, id)
}
func (r *StoreOperationRepository) UpdateStatus(ctx context.Context, id string, status operations.Status, operationError string) error {
	return r.store.UpdateOperationStatus(ctx, id, status, operationError)
}

var _ operations.Repository = (*StoreOperationRepository)(nil)
