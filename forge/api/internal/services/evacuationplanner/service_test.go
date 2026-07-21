package evacuationplanner

import (
	"context"
	"errors"
	"testing"

	"gamepanel/forge/internal/domain"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

// mockScheduler implements schedulersvc.Service for test purposes.
type mockScheduler struct {
	filterNodesFn func(context.Context, domain.PlacementRequest, []store.Node) ([]store.Node, error)
	scoreNodesFn  func(context.Context, domain.PlacementRequest, []store.Node) ([]schedulersvc.NodeScore, error)
}

func (m *mockScheduler) PlaceServer(_ context.Context, _ domain.PlacementRequest) (domain.PlacementDecision, error) {
	return domain.PlacementDecision{}, errors.New("not implemented")
}

func (m *mockScheduler) FilterNodes(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
	if m.filterNodesFn != nil {
		return m.filterNodesFn(ctx, req, nodes)
	}
	return nodes, nil
}

func (m *mockScheduler) ScoreNodes(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]schedulersvc.NodeScore, error) {
	if m.scoreNodesFn != nil {
		return m.scoreNodesFn(ctx, req, nodes)
	}
	return nil, errors.New("no scores")
}

func (m *mockScheduler) PlaceReplicas(_ context.Context, _ domain.PlaceReplicasRequest) ([]domain.PlacementReason, error) {
	return nil, errors.New("not implemented")
}

func (m *mockScheduler) ScaleReplicas(_ context.Context, _ domain.ScaleRequest) ([]domain.PlacementReason, error) {
	return nil, errors.New("not implemented")
}

func (m *mockScheduler) ReplaceFailedInstance(_ context.Context, _ domain.ReplaceFailedInstanceRequest) (*domain.PlacementReason, error) {
	return nil, errors.New("not implemented")
}

// mockExecutor implements MigrationExecutor for test purposes.
type mockExecutor struct {
	cancelFn func(context.Context, string) error
}

func (m *mockExecutor) CreateEvacuationMigration(_ context.Context, _, _, _ string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockExecutor) PrepareEvacuationMigration(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (m *mockExecutor) ExecuteEvacuationMigration(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

func (m *mockExecutor) CancelEvacuationMigration(ctx context.Context, id string) error {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, id)
	}
	return nil
}

func (m *mockExecutor) EvacuationMigrationStatus(_ context.Context, _ string) (string, error) {
	return "", errors.New("not implemented")
}

func TestEvacuationMigrationOutcomeOnlyCompletesTerminalMigrations(t *testing.T) {
	tests := map[string]string{
		string(store.MigrationStatusPlanned):      "pending",
		string(store.MigrationStatusPreparing):    "pending",
		string(store.MigrationStatusTransferring): "pending",
		string(store.MigrationStatusRestoring):    "pending",
		string(store.MigrationStatusCompleted):    "completed",
		string(store.MigrationStatusFailed):       "failed",
		string(store.MigrationStatusCancelled):    "failed",
	}
	for status, want := range tests {
		t.Run(status, func(t *testing.T) {
			if got := evacuationMigrationOutcome(status); got != want {
				t.Fatalf("evacuationMigrationOutcome(%q) = %q, want %q", status, got, want)
			}
		})
	}
}

func TestEvacuationPlanFinishedRequiresEveryItemToBeTerminal(t *testing.T) {
	tests := []struct {
		name  string
		items []store.EvacuationItem
		want  bool
	}{
		{name: "empty", want: true},
		{name: "completed and failed", items: []store.EvacuationItem{{Status: "completed"}, {Status: "failed"}}, want: true},
		{name: "pending", items: []store.EvacuationItem{{Status: "pending"}}, want: false},
		{name: "preparing", items: []store.EvacuationItem{{Status: "preparing"}}, want: false},
		{name: "running", items: []store.EvacuationItem{{Status: "running"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evacuationPlanFinished(tt.items); got != tt.want {
				t.Fatalf("evacuationPlanFinished() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestEvacuationPlanStatusReflectsPlanningOnly(t *testing.T) {
	tests := []struct {
		name  string
		items []PlanItem
		want  store.EvacuationPlanStatus
	}{
		{
			name:  "eligible targets remain pending execution",
			items: []PlanItem{{EvacuationItem: store.EvacuationItem{Eligible: true}}},
			want:  store.EvacuationPlanStatusPending,
		},
		{
			name: "empty plan is completed (nothing to evacuate)",
			want: store.EvacuationPlanStatusCompleted,
		},
		{
			name:  "ineligible target fails planning",
			items: []PlanItem{{EvacuationItem: store.EvacuationItem{Eligible: false}}},
			want:  store.EvacuationPlanStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evacuationPlanStatus(tt.items); got != tt.want {
				t.Fatalf("evacuationPlanStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecutePlanNilStore(t *testing.T) {
	svc := &Service{}
	_, err := svc.ExecutePlan(context.Background(), "plan-1")
	if err == nil || err.Error() != "store unavailable" {
		t.Fatalf("expected 'store unavailable', got %v", err)
	}
}

func TestExecutePlanNilExecutor(t *testing.T) {
	svc := &Service{store: &store.Store{}}
	_, err := svc.ExecutePlan(context.Background(), "plan-1")
	if err == nil || err.Error() != "migration executor unavailable" {
		t.Fatalf("expected 'migration executor unavailable', got %v", err)
	}
}

func TestCancelPlanNilStore(t *testing.T) {
	svc := &Service{}
	_, err := svc.CancelPlan(context.Background(), "plan-1")
	if err == nil || err.Error() != "store unavailable" {
		t.Fatalf("expected 'store unavailable', got %v", err)
	}
}

func TestCancelPlanNilExecutor(t *testing.T) {
	svc := &Service{store: &store.Store{}}
	_, err := svc.CancelPlan(context.Background(), "plan-1")
	if err == nil || err.Error() != "migration executor unavailable" {
		t.Fatalf("expected 'migration executor unavailable', got %v", err)
	}
}

func TestFindCandidatesSchedulerFilterError(t *testing.T) {
	svc := &Service{
		scheduler: &mockScheduler{
			filterNodesFn: func(_ context.Context, _ domain.PlacementRequest, _ []store.Node) ([]store.Node, error) {
				return nil, errors.New("scheduler unavailable")
			},
		},
	}
	node, _, reason := svc.FindCandidates(context.Background(), store.Server{}, store.Node{}, nil)
	if node.ID != "" {
		t.Fatal("expected no candidate node")
	}
	if reason != "scheduler unavailable" {
		t.Fatalf("reason = %q, want %q", reason, "scheduler unavailable")
	}
}

func TestFindCandidatesNoEligibleNodes(t *testing.T) {
	sourceID := "source-1"
	svc := &Service{
		scheduler: &mockScheduler{
			filterNodesFn: func(_ context.Context, _ domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
				return nodes, nil
			},
		},
	}
	node, _, reason := svc.FindCandidates(
		context.Background(),
		store.Server{},
		store.Node{ID: sourceID},
		[]store.Node{{ID: sourceID}},
	)
	if node.ID != "" {
		t.Fatal("expected no candidate node when source is the only node")
	}
	if reason != "no eligible candidate nodes" {
		t.Fatalf("reason = %q, want %q", reason, "no eligible candidate nodes")
	}
}

func TestFindCandidatesScoringError(t *testing.T) {
	svc := &Service{
		scheduler: &mockScheduler{
			filterNodesFn: func(_ context.Context, _ domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
				return nodes, nil
			},
			scoreNodesFn: func(_ context.Context, _ domain.PlacementRequest, _ []store.Node) ([]schedulersvc.NodeScore, error) {
				return nil, errors.New("scoring unavailable")
			},
		},
	}
	node, _, reason := svc.FindCandidates(
		context.Background(),
		store.Server{},
		store.Node{ID: "source-1"},
		[]store.Node{{ID: "source-1"}, {ID: "candidate-1"}},
	)
	if node.ID != "" {
		t.Fatal("expected no candidate node when scoring fails")
	}
	if reason != "no scored candidate nodes" {
		t.Fatalf("reason = %q, want %q", reason, "no scored candidate nodes")
	}
}

func TestFindCandidatesScoringReturnsEmpty(t *testing.T) {
	svc := &Service{
		scheduler: &mockScheduler{
			filterNodesFn: func(_ context.Context, _ domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
				return nodes, nil
			},
			scoreNodesFn: func(_ context.Context, _ domain.PlacementRequest, _ []store.Node) ([]schedulersvc.NodeScore, error) {
				return []schedulersvc.NodeScore{}, nil
			},
		},
	}
	node, _, reason := svc.FindCandidates(
		context.Background(),
		store.Server{},
		store.Node{ID: "source-1"},
		[]store.Node{{ID: "source-1"}, {ID: "candidate-1"}},
	)
	if node.ID != "" {
		t.Fatal("expected no candidate node when scores are empty")
	}
	if reason != "no scored candidate nodes" {
		t.Fatalf("reason = %q, want %q", reason, "no scored candidate nodes")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "first non-empty in middle", values: []string{"", "hello", "world"}, want: "hello"},
		{name: "all empty", values: []string{"", "", ""}, want: ""},
		{name: "first non-empty at start", values: []string{"first", "second"}, want: "first"},
		{name: "single value", values: []string{"only"}, want: "only"},
		{name: "empty slice", values: []string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.values...); got != tt.want {
				t.Fatalf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	t.Run("nil service returns empty", func(t *testing.T) {
		var svc *Service
		if m := svc.Metrics(); m != (Metrics{}) {
			t.Fatalf("expected empty metrics, got %+v", m)
		}
	})
}

func TestSetMigrationExecutorNilService(t *testing.T) {
	var svc *Service
	svc.SetMigrationExecutor(&mockExecutor{})
}
