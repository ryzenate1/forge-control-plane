package server

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueLoadJournalResetsRunningToPending(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "operations.db")
	// Create a queue and manually insert a running operation
	q1, err := NewPersistentOperationQueue(journal, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Directly insert a running operation into DB to simulate crash mid-execution
	_, err = q1.db.Exec(`INSERT INTO beacon_operations(id, command_id, server_id, type, status, error, created_at, started_at)
		VALUES(?,?,?,?,?,?,?,?)`, "op-running-1", "cmd-running-1", "srv-1", string(OpStart), string(StatusRunning), "", time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	q1.Shutdown()

	// Reload journal — loadJournal should reset running -> pending
	q2, err := NewPersistentOperationQueue(journal, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Shutdown()
	status, err := q2.GetStatus("op-running-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusPending {
		t.Fatalf("running operation should be reset to pending after crash, got %s", status.Status)
	}
	if !status.StartedAt.IsZero() {
		t.Fatalf("pending operation should have zero StartedAt after reset, got %v", status.StartedAt)
	}
	if status.Error != "" {
		t.Fatalf("pending operation should have empty error after reset, got %q", status.Error)
	}
}

func TestQueueStartRequeuesPendingAfterCrash(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "operations.db")
	var executed atomic.Int32
	q1, err := NewPersistentOperationQueue(journal, 1, func(ctx context.Context, op *Operation) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	op, err := q1.EnqueueCommand(context.Background(), "cmd-pending-1", "srv-1", OpRestart)
	if err != nil {
		t.Fatal(err)
	}
	// Shutdown without letting handler complete fully — simulate crash
	// To simulate crash before completion, we insert pending directly and not start handler
	// Instead create a new queue with execution counting, then Start should requeue pending
	q1.Shutdown()
	// Verify DB still has pending
	q2, err := NewPersistentOperationQueue(journal, 1, func(ctx context.Context, op *Operation) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	status, _ := q2.GetStatus(op.ID)
	if status.Status != StatusPending {
		t.Fatalf("expected pending before Start, got %s", status.Status)
	}
	executed.Store(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q2.Start(ctx)
	defer q2.Shutdown()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := q2.GetStatus(op.ID)
		if s.Status == StatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s, _ := q2.GetStatus(op.ID)
	if s.Status != StatusCompleted {
		t.Fatalf("pending operation was not requeued after Start, status=%s", s.Status)
	}
	if executed.Load() != 1 {
		t.Fatalf("expected handler executed once after requeue, got %d", executed.Load())
	}
}

func TestQueueJournalDurabilityAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "operations.db")
	q1, err := NewPersistentOperationQueue(journal, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	ops := []*Operation{}
	for i := 0; i < 3; i++ {
		op, err := q1.Enqueue(context.Background(), "srv-1", OpStart)
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, op)
	}
	// Mark one as running manually to simulate crash
	q1.mu.Lock()
	running := q1.operations[ops[0].ID]
	running.Status = StatusRunning
	running.StartedAt = time.Now().UTC()
	_ = q1.persist(running)
	q1.mu.Unlock()
	q1.Shutdown()

	q2, err := NewPersistentOperationQueue(journal, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Shutdown()
	for _, op := range ops {
		s, _ := q2.GetStatus(op.ID)
		if s.Status != StatusPending {
			t.Fatalf("op %s after reload should be pending, got %s", op.ID, s.Status)
		}
	}
	// Ensure Start would requeue all three
	if len(q2.ListPendingByServer("srv-1")) != 3 {
		t.Fatalf("expected 3 pending after recovery, got %d", len(q2.ListPendingByServer("srv-1")))
	}
}
