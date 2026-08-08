package queue

import (
	"context"
	"encoding/json"
	"fmt"
)

type Worker[T JobArgs] interface {
	Work(ctx context.Context, job *Job[T]) error
}

type WorkerDefaults[T JobArgs] struct{}

func (w WorkerDefaults[T]) Work(ctx context.Context, job *Job[T]) error { return nil }

type WorkFunctionWrapper[T JobArgs] struct {
	kind     string
	workFunc func(ctx context.Context, job *Job[T]) error
}

func (w *WorkFunctionWrapper[T]) Kind() string { return w.kind }

func (w *WorkFunctionWrapper[T]) Work(ctx context.Context, job *Job[T]) error {
	return w.workFunc(ctx, job)
}

func WorkFunc[T JobArgs](fn func(ctx context.Context, job *Job[T]) error) Worker[T] {
	var args T
	return &WorkFunctionWrapper[T]{kind: args.Kind(), workFunc: fn}
}

type workerInfo struct {
	jobArgs    JobArgs
	workFunc   func(context.Context, *JobRow) error
	workerKind string
}

type Workers struct {
	workersMap map[string]workerInfo
}

func NewWorkers() *Workers {
	return &Workers{
		workersMap: make(map[string]workerInfo),
	}
}

func AddWorker[T JobArgs](workers *Workers, worker Worker[T]) {
	if err := AddWorkerSafely(workers, worker); err != nil {
		panic(err)
	}
}

func AddWorkerSafely[T JobArgs](workers *Workers, worker Worker[T]) error {
	var args T
	kind := args.Kind()
	if _, ok := workers.workersMap[kind]; ok {
		return fmt.Errorf("worker for kind %q is already registered", kind)
	}

	// Validate kind format
	if kind == "" {
		return fmt.Errorf("job kind must not be empty")
	}

	workers.workersMap[kind] = workerInfo{
		jobArgs:    args,
		workerKind: kind,
		workFunc: func(ctx context.Context, row *JobRow) error {
			job := &Job[T]{JobRow: row}
			var args T
			if err := json.Unmarshal(row.EncodedArgs, &args); err != nil {
				return fmt.Errorf("failed to unmarshal job args for kind %q: %w", kind, err)
			}
			job.Args = args
			return worker.Work(ctx, job)
		},
	}
	return nil
}

func (w *Workers) Lookup(kind string) (workerInfo, bool) {
	info, ok := w.workersMap[kind]
	return info, ok
}
