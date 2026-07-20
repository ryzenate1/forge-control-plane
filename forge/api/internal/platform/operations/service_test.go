package operations

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct{ operation Operation }
func (r *memoryRepository) Create(_ context.Context, request Request) (Operation, error) { r.operation = Operation{ID: "operation-1", Kind: request.Kind, ResourceID: request.ResourceID, Status: StatusQueued}; return r.operation, nil }
func (r *memoryRepository) Get(_ context.Context, _ string) (Operation, error) { return r.operation, nil }
func (r *memoryRepository) UpdateStatus(_ context.Context, _ string, status Status, message string) error { r.operation.Status, r.operation.Error = status, message; return nil }

type testDriver struct { kind string; err error }
func (d testDriver) Kind() string { return d.kind }
func (d testDriver) Execute(context.Context, Operation) error { return d.err }

func TestServicePersistsBeforeExecutingDriver(t *testing.T) {
	repository := &memoryRepository{}
	service, err := NewService(repository, testDriver{kind: "workload.deploy"})
	if err != nil { t.Fatal(err) }
	operation, err := service.Dispatch(context.Background(), Request{Kind: "workload.deploy", ResourceType: "workload", ResourceID: "w1"})
	if err != nil { t.Fatal(err) }
	if operation.Status != StatusQueued { t.Fatalf("status = %s, want queued", operation.Status) }
	if err := service.Execute(context.Background(), operation.ID); err != nil { t.Fatal(err) }
	if repository.operation.Status != StatusSucceeded { t.Fatalf("status = %s, want succeeded", repository.operation.Status) }
}

func TestServiceRecordsDriverFailure(t *testing.T) {
	repository := &memoryRepository{operation: Operation{ID: "operation-1", Kind: "workload.deploy"}}
	service, err := NewService(repository, testDriver{kind: "workload.deploy", err: errors.New("runtime unavailable")})
	if err != nil { t.Fatal(err) }
	if err := service.Execute(context.Background(), "operation-1"); err == nil { t.Fatal("expected driver error") }
	if repository.operation.Status != StatusFailed { t.Fatalf("status = %s, want failed", repository.operation.Status) }
}
