package store

import (
	"context"
	"testing"
)

func TestDurableOperationsSchemaMatchesWorkers(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	var operationID string
	err := s.db.QueryRow(ctx, `
		INSERT INTO operations (
			id, kind, resource_type, resource_id, status, input, started_at
		)
		VALUES (
			gen_random_uuid(), 'server.start', 'server', 'server-1',
			'running', '{"signal":"start"}'::jsonb, NOW()
		)
		RETURNING id::text
	`).Scan(&operationID)
	if err != nil {
		t.Fatalf("insert operation using worker columns: %v", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO operation_steps (
			id, operation_id, name, position, status, max_attempts
		)
		VALUES ('step-' || $1, $1::uuid, 'execute', 0, 'running', 3)
	`, operationID); err != nil {
		t.Fatalf("insert operation step: %v", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO operation_attempts (
			id, operation_step_id, attempt, status, worker_id
		)
		VALUES ('attempt-' || $1, 'step-' || $1, 1, 'running', 'worker-1')
	`, operationID); err != nil {
		t.Fatalf("insert operation attempt: %v", err)
	}
}
