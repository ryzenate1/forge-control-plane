package reconciliation

import (
	"context"
	"testing"

	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type testSource struct{ candidates []Candidate }

func (s testSource) Pending(context.Context, int) ([]Candidate, error) { return s.candidates, nil }

type testPlanner struct{}

func (testPlanner) Plan(_ context.Context, candidate Candidate) (operations.Request, error) {
	return operations.Request{Kind: "workload.reconcile", ResourceID: candidate.Workload.ID, IdempotencyKey: candidate.Workload.ID + "-" + string(rune(candidate.Workload.DesiredGeneration))}, nil
}

type testDispatcher struct{ count int }

func (d *testDispatcher) Dispatch(_ context.Context, request operations.Request) (operations.Operation, error) {
	d.count++
	return operations.Operation{ID: request.ResourceID, Status: operations.StatusQueued, DesiredGeneration: request.DesiredGeneration}, nil
}

func TestReconcileDispatchesDesiredStateGaps(t *testing.T) {
	dispatcher := &testDispatcher{}
	service, err := New(testSource{candidates: []Candidate{{Workload: workloads.Workload{ID: "w1", DesiredGeneration: 2, ObservedGeneration: 1}}}}, testPlanner{}, dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := service.Reconcile(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || dispatcher.count != 1 || operations[0].DesiredGeneration != 2 {
		t.Fatalf("unexpected reconciliation result: %#v", operations)
	}
}
