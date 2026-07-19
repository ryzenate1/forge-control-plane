package commands

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueEnqueueAndProcess(t *testing.T) {
	var processed int32
	handler := func(ctx context.Context, op *Operation) error {
		atomic.AddInt32(&processed, 1)
		return nil
	}

	q := NewQueue(1, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	op, err := q.Enqueue(ctx, "srv-1", Start)
	if err != nil {
		t.Fatal(err)
	}
	if op.ServerID != "srv-1" || op.Type != Start {
		t.Fatalf("unexpected op: %+v", op)
	}

	time.Sleep(100 * time.Millisecond)

	status, err := q.GetStatus(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", status.Status)
	}
	if atomic.LoadInt32(&processed) != 1 {
		t.Fatal("handler not called")
	}
}

func TestQueueHandlerError(t *testing.T) {
	handler := func(ctx context.Context, op *Operation) error {
		return errors.New("boom")
	}

	q := NewQueue(1, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	op, err := q.Enqueue(ctx, "srv-1", Stop)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	status, err := q.GetStatus(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusFailed {
		t.Fatalf("expected failed, got %s", status.Status)
	}
	if status.Error != "boom" {
		t.Fatalf("expected error 'boom', got %q", status.Error)
	}
}

func TestQueueConcurrency(t *testing.T) {
	q := NewQueue(5, nil)
	if q.concurrency != 4 {
		t.Fatalf("expected max concurrency 4, got %d", q.concurrency)
	}

	q2 := NewQueue(0, nil)
	if q2.concurrency != 1 {
		t.Fatalf("expected min concurrency 1, got %d", q2.concurrency)
	}
}

func TestQueueListByServer(t *testing.T) {
	q := NewQueue(1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	q.Enqueue(ctx, "srv-1", Start)
	q.Enqueue(ctx, "srv-1", Stop)
	q.Enqueue(ctx, "srv-2", Restart)

	time.Sleep(150 * time.Millisecond)

	ops := q.ListByServer("srv-1")
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops for srv-1, got %d", len(ops))
	}

	ops2 := q.ListByServer("srv-2")
	if len(ops2) != 1 {
		t.Fatalf("expected 1 op for srv-2, got %d", len(ops2))
	}

	ops3 := q.ListByServer("srv-3")
	if len(ops3) != 0 {
		t.Fatalf("expected 0 ops for srv-3, got %d", len(ops3))
	}
}

func TestQueueGetStatusNotFound(t *testing.T) {
	q := NewQueue(1, nil)
	_, err := q.GetStatus("nonexistent")
	if !errors.Is(err, ErrOperationNotFound) {
		t.Fatalf("expected ErrOperationNotFound, got %v", err)
	}
}

func TestQueueShutdown(t *testing.T) {
	blockCh := make(chan struct{})
	handler := func(ctx context.Context, op *Operation) error {
		<-blockCh
		return nil
	}

	q := NewQueue(1, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	q.Enqueue(ctx, "srv-1", Start)
	q.Enqueue(ctx, "srv-1", Stop)
	q.Enqueue(ctx, "srv-1", Restart)

	time.Sleep(50 * time.Millisecond)
	close(blockCh)
	q.Shutdown()

	ops := q.ListByServer("srv-1")
	cancelled := 0
	for _, op := range ops {
		if op.Status == StatusCancelled {
			cancelled++
		}
	}
	if cancelled == 0 {
		t.Fatal("expected at least one cancelled operation after shutdown")
	}
}

func TestQueueAllOperationTypes(t *testing.T) {
	types := []Type{Start, Stop, Restart, Install, Reinstall}
	q := NewQueue(1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	for _, ot := range types {
		op, err := q.Enqueue(ctx, "srv-1", ot)
		if err != nil {
			t.Fatal(err)
		}
		if op.Type != ot {
			t.Fatalf("expected type %s, got %s", ot, op.Type)
		}
	}
}

func TestPersistentQueueReplaysAndDeduplicatesCommands(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "operations.db")
	q1, err := NewPersistentQueue(journal, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	op, err := q1.EnqueueCommand(context.Background(), "command-1", "srv-1", Restart)
	if err != nil {
		t.Fatal(err)
	}
	q1.Shutdown()

	var executions atomic.Int32
	q2, err := NewPersistentQueue(journal, 1, func(context.Context, *Operation) error {
		executions.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	q2.Start(context.Background())
	defer q2.Shutdown()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, getErr := q2.GetStatus(op.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if status.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation was not replayed: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	duplicate, err := q2.EnqueueCommand(context.Background(), "command-1", "srv-1", Restart)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != op.ID {
		t.Fatalf("duplicate command created a new operation: %s != %s", duplicate.ID, op.ID)
	}
	if executions.Load() != 1 {
		t.Fatalf("command executed %d times", executions.Load())
	}
}
