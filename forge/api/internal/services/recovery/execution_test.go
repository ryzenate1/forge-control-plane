package recovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// mockCoordinatorStore implements coordinatorStore for unit tests.
type mockCoordinatorStore struct {
	plans map[string]*store.RecoveryPlan
	items map[string][]store.RecoveryItem
	nodes map[string]store.Node

	// injectable behaviour
	startPlanErr      error
	updateStatusErr   error
	getPlanErr        error
	createItemPlanID  string
	createItem        store.RecoveryItem
	createItemErr     error
}

func newMockStore() *mockCoordinatorStore {
	return &mockCoordinatorStore{
		plans: make(map[string]*store.RecoveryPlan),
		items: make(map[string][]store.RecoveryItem),
		nodes: make(map[string]store.Node),
	}
}

func (m *mockCoordinatorStore) CreateRecoveryPlan(_ context.Context, nodeID, reason string) (store.RecoveryPlan, error) {
	id := fmt.Sprintf("plan-%d", len(m.plans)+1)
	now := time.Now().UTC()
	p := store.RecoveryPlan{
		ID: id, NodeID: nodeID, Status: store.RecoveryPlanStatusPending,
		Reason: reason, CreatedAt: now, UpdatedAt: now,
	}
	m.plans[id] = &p
	m.items[id] = []store.RecoveryItem{}
	return p, nil
}

func (m *mockCoordinatorStore) UpdateRecoveryPlanStatus(_ context.Context, planID string, status store.RecoveryPlanStatus, reason string) (store.RecoveryPlan, error) {
	if m.updateStatusErr != nil {
		return store.RecoveryPlan{}, m.updateStatusErr
	}
	p, ok := m.plans[planID]
	if !ok {
		return store.RecoveryPlan{}, fmt.Errorf("plan %s not found", planID)
	}
	p.Status = status
	p.Reason = reason
	p.UpdatedAt = time.Now().UTC()
	items := m.items[planID]
	p.Items = items
	return *p, nil
}

func (m *mockCoordinatorStore) GetRecoveryPlan(_ context.Context, planID string) (store.RecoveryPlan, error) {
	if m.getPlanErr != nil {
		return store.RecoveryPlan{}, m.getPlanErr
	}
	p, ok := m.plans[planID]
	if !ok {
		return store.RecoveryPlan{}, fmt.Errorf("plan %s not found", planID)
	}
	items := m.items[planID]
	p.Items = items
	return *p, nil
}

func (m *mockCoordinatorStore) ListRecoveryPlans(_ context.Context) ([]store.RecoveryPlan, error) {
	var out []store.RecoveryPlan
	for _, p := range m.plans {
		p.Items = m.items[p.ID]
		out = append(out, *p)
	}
	return out, nil
}

func (m *mockCoordinatorStore) GetNode(_ context.Context, nodeID string) (store.Node, error) {
	n, ok := m.nodes[nodeID]
	if !ok {
		return store.Node{}, fmt.Errorf("node %s not found", nodeID)
	}
	return n, nil
}

func (m *mockCoordinatorStore) ListServersForNode(_ context.Context, _ string) ([]store.Server, error) {
	return nil, nil
}

func (m *mockCoordinatorStore) CreateRecoveryItem(_ context.Context, planID string, item store.RecoveryItem) (store.RecoveryItem, error) {
	if m.createItemErr != nil {
		return store.RecoveryItem{}, m.createItemErr
	}
	item.ID = fmt.Sprintf("item-%d", len(m.items[planID])+1)
	item.PlanID = planID
	m.items[planID] = append(m.items[planID], item)
	return item, nil
}

func (m *mockCoordinatorStore) UpdateRecoveryItemStatus(_ context.Context, itemID string, status store.RecoveryItemStatus, reason string) (store.RecoveryItem, error) {
	for _, items := range m.items {
		for i, item := range items {
			if item.ID == itemID {
				item.Status = string(status)
				item.Reason = reason
				items[i] = item
				return item, nil
			}
		}
	}
	return store.RecoveryItem{}, fmt.Errorf("item %s not found", itemID)
}

func (m *mockCoordinatorStore) UpdateRecoveryItemsForPlanStatus(_ context.Context, planID string, _ store.RecoveryItemStatus, _ string) error {
	return nil
}

func (m *mockCoordinatorStore) StartRecoveryPlan(_ context.Context, planID string) (store.RecoveryPlan, error) {
	if m.startPlanErr != nil {
		return store.RecoveryPlan{}, m.startPlanErr
	}
	p, ok := m.plans[planID]
	if !ok {
		return store.RecoveryPlan{}, fmt.Errorf("plan %s not found", planID)
	}
	p.Status = store.RecoveryPlanStatusExecuting
	for i, item := range m.items[planID] {
		if item.Status == string(store.RecoveryItemStatusPlanned) {
			item.Status = string(store.RecoveryItemStatusExecuting)
			m.items[planID][i] = item
		}
	}
	p.Items = m.items[planID]
	return *p, nil
}

func (m *mockCoordinatorStore) LatestVerifiedRecoveryBackup(_ context.Context, _ string) (store.Backup, error) {
	return store.Backup{}, fmt.Errorf("no backup")
}

func (m *mockCoordinatorStore) CreateMigration(_ context.Context, _ store.CreateMigrationRequest) (store.Migration, error) {
	return store.Migration{}, nil
}

func (m *mockCoordinatorStore) UpdateMigrationStatus(_ context.Context, _ string, _ store.MigrationStatus, _ string) (store.Migration, error) {
	return store.Migration{}, nil
}

func (m *mockCoordinatorStore) UpdateServerGeneration(_ context.Context, _ string, _ int64, _ *time.Time) error {
	return nil
}

func (m *mockCoordinatorStore) GetServer(_ context.Context, serverID string) (store.Server, error) {
	return store.Server{ID: serverID, ActualState: store.ServerActualStateRunning}, nil
}

func (m *mockCoordinatorStore) ListRecoveryItemsByStatus(_ context.Context, statuses ...string) ([]store.RecoveryItem, error) {
	var items []store.RecoveryItem
	for _, planItems := range m.items {
		for _, item := range planItems {
			for _, s := range statuses {
				if item.Status == s {
					items = append(items, item)
					break
				}
			}
		}
	}
	return items, nil
}



type recordingMigrationExecutor struct {
	calls      []string
	prepareErr error
	executeErr error
}

func (e *recordingMigrationExecutor) PrepareMigration(_ context.Context, id string) (store.Migration, error) {
	e.calls = append(e.calls, "prepare:"+id)
	return store.Migration{ID: id}, e.prepareErr
}

func (e *recordingMigrationExecutor) ExecuteMigration(_ context.Context, id string) (store.Migration, error) {
	e.calls = append(e.calls, "execute:"+id)
	return store.Migration{ID: id}, e.executeErr
}

func (e *recordingMigrationExecutor) GetMigration(_ context.Context, id string) (store.Migration, error) {
	return store.Migration{ID: id}, nil
}

func (e *recordingMigrationExecutor) CancelMigration(_ context.Context, id string) (store.Migration, error) {
	return store.Migration{ID: id}, nil
}

func (e *recordingMigrationExecutor) MarkFailed(_ context.Context, id, _ string) (store.Migration, error) {
	return store.Migration{ID: id}, nil
}

var _ MigrationExecutor = (*recordingMigrationExecutor)(nil)

type recordingBackupRestoreExecutor struct {
	calls []store.RecoveryItem
	err   error
}

func (e *recordingBackupRestoreExecutor) VerifyAndRestore(_ context.Context, item store.RecoveryItem) error {
	e.calls = append(e.calls, item)
	return e.err
}

var _ BackupRestoreExecutor = (*recordingBackupRestoreExecutor)(nil)

func TestNewWithMigrationExecutorRegistersExecutor(t *testing.T) {
	executor := &recordingMigrationExecutor{}
	coordinator := NewWithMigrationExecutor(nil, nil, nil, executor)

	if coordinator.migrationExecutor() != executor {
		t.Fatal("migration executor was not registered")
	}
}

func TestBackupRestoreExecutorRegistersWithoutMigrationExecutor(t *testing.T) {
	restore := &recordingBackupRestoreExecutor{}
	coordinator := New(nil, nil, nil)
	coordinator.SetBackupRestoreExecutor(restore)

	if coordinator.backupRestoreExecutor() != restore {
		t.Fatal("backup restore executor was not registered")
	}
	if coordinator.migrationExecutor() != nil {
		t.Fatal("backup recovery must not require a live migration executor")
	}
}

func TestDaemonBackupRestoreRejectsUnverifiedSource(t *testing.T) {
	executor := &DaemonBackupRestoreExecutor{store: &store.Store{}, daemon: daemon.NewClient()}
	err := executor.VerifyAndRestore(context.Background(), store.RecoveryItem{
		ID: "item-1", ServerID: "server-1", TargetNodeID: "target-1",
	})
	if err == nil || err.Error() != "recovery item has no verified backup restore source" {
		t.Fatalf("VerifyAndRestore error = %v, want missing-source error", err)
	}
}

func TestExecuteItemPreparesThenExecutesMigration(t *testing.T) {
	executor := &recordingMigrationExecutor{}
	coordinator := &Coordinator{}
	item := store.RecoveryItem{ID: "item-1", MigrationID: "migration-1"}

	if err := coordinator.executeItem(context.Background(), executor, item); err != nil {
		t.Fatalf("executeItem returned an error: %v", err)
	}
	if want := []string{"prepare:migration-1", "execute:migration-1"}; !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("executor calls = %v, want %v", executor.calls, want)
	}
}

func TestExecuteItemDoesNotExecuteWhenPreparationFails(t *testing.T) {
	prepareErr := errors.New("target allocation unavailable")
	executor := &recordingMigrationExecutor{prepareErr: prepareErr}
	coordinator := &Coordinator{}
	item := store.RecoveryItem{ID: "item-1", MigrationID: "migration-1"}

	err := coordinator.executeItem(context.Background(), executor, item)
	if !errors.Is(err, prepareErr) {
		t.Fatalf("executeItem error = %v, want wrapped %v", err, prepareErr)
	}
	if want := []string{"prepare:migration-1"}; !reflect.DeepEqual(executor.calls, want) {
		t.Fatalf("executor calls = %v, want %v", executor.calls, want)
	}
}

func TestExecuteItemRequiresMigrationRecord(t *testing.T) {
	executor := &recordingMigrationExecutor{}
	coordinator := &Coordinator{}

	err := coordinator.executeItem(context.Background(), executor, store.RecoveryItem{ID: "item-1"})
	if err == nil || err.Error() != "recovery item has no migration" {
		t.Fatalf("executeItem error = %v, want missing-migration error", err)
	}
	if len(executor.calls) != 0 {
		t.Fatalf("executor was called for an item without a migration: %v", executor.calls)
	}
}

func TestEvaluateNode(t *testing.T) {
	s := newMockStore()
	s.nodes["node-1"] = store.Node{ID: "node-1", HeartbeatState: "offline", ActualState: "offline"}
	s.nodes["node-2"] = store.Node{ID: "node-2", HeartbeatState: "online", ActualState: "online"}
	c := &Coordinator{store: s}

	t.Run("offline node is accepted", func(t *testing.T) {
		n, err := c.EvaluateNode(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("EvaluateNode returned error: %v", err)
		}
		if n.ID != "node-1" {
			t.Fatalf("got node %s, want node-1", n.ID)
		}
	})

	t.Run("online node is rejected", func(t *testing.T) {
		_, err := c.EvaluateNode(context.Background(), "node-2")
		if err == nil || err.Error() != "node is not offline" {
			t.Fatalf("EvaluateNode error = %v, want 'node is not offline'", err)
		}
	})

	t.Run("empty nodeID returns error", func(t *testing.T) {
		_, err := c.EvaluateNode(context.Background(), "")
		if err == nil || err.Error() != "nodeId is required" {
			t.Fatalf("EvaluateNode error = %v, want 'nodeId is required'", err)
		}
	})

	t.Run("nil coordinator returns zero-value", func(t *testing.T) {
		var nilCoord *Coordinator
		n, err := nilCoord.EvaluateNode(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil coordinator")
		}
		if n.ID != "" {
			t.Fatal("expected empty node from nil coordinator")
		}
	})
}

func TestIdentifyAffectedServers(t *testing.T) {
	t.Run("nil coordinator returns error", func(t *testing.T) {
		var nilCoord *Coordinator
		_, err := nilCoord.IdentifyAffectedServers(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil coordinator")
		}
	})
}

func TestFinishBackupRestorePlan(t *testing.T) {
	t.Run("all items restored completes plan", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusRestored),
		})
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-2", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusSkipped),
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		if result.Status != store.RecoveryPlanStatusRestored {
			t.Fatalf("expected plan restored, got %v", result.Status)
		}
	})

	t.Run("all items skipped completes with no restore", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusSkipped),
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		if result.Status != store.RecoveryPlanStatusCompleted {
			t.Fatalf("expected plan completed, got %v", result.Status)
		}
		if result.Reason != "no verified backup restore sources were available" {
			t.Fatalf("unexpected reason: %s", result.Reason)
		}
	})

	t.Run("item in non-terminal status does not complete plan", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusPlanned),
		})
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-2", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusRestored),
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		if result.Status == store.RecoveryPlanStatusRestored || result.Status == store.RecoveryPlanStatusCompleted {
			t.Fatalf("plan should not complete with non-terminal item, got %v", result.Status)
		}
	})

	t.Run("failed item fails the plan", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1",
			Status: string(store.RecoveryItemStatusFailed),
			Reason: "disk full",
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		if result.Status != store.RecoveryPlanStatusFailed {
			t.Fatalf("expected plan failed, got %v", result.Status)
		}
	})

	t.Run("cancelled item is treated as terminal", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusCancelled),
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		// No restored items, so should complete without restore
		if result.Status != store.RecoveryPlanStatusCompleted {
			t.Fatalf("expected plan completed, got %v", result.Status)
		}
	})

	t.Run("completed item counts as restored", func(t *testing.T) {
		s := newMockStore()
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1",
			Status: string(store.RecoveryItemStatusCompleted),
		})
		c := &Coordinator{store: s}

		result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("finishBackupRestorePlan error = %v", err)
		}
		if result.Status != store.RecoveryPlanStatusRestored {
			t.Fatalf("expected plan restored, got %v", result.Status)
		}
	})

}

func TestGetPlanNilCoordinator(t *testing.T) {
	t.Run("nil coordinator returns error", func(t *testing.T) {
		var nilCoord *Coordinator
		_, err := nilCoord.GetPlan(context.Background(), "plan-1")
		if err == nil {
			t.Fatal("expected error from nil coordinator")
		}
	})
}

func TestMetricsNilCoordinator(t *testing.T) {
	var nilCoord *Coordinator
	m := nilCoord.Metrics()
	if m.RecoveryPlansTotal != 0 || m.RecoveryItemsTotal != 0 || m.RecoveryFailuresTotal != 0 {
		t.Fatal("expected zero metrics from nil coordinator")
	}
}

func TestReconcileRecoveryAcknowledgmentServerRunning(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "target-1",
		Status:    string(store.RecoveryItemStatusAwaitingBeacon),
		UpdatedAt: time.Now().UTC(),
	})
	c := &Coordinator{store: s}

	err := c.ReconcileRecoveryAcknowledgment(context.Background())
	if err != nil {
		t.Fatalf("ReconcileRecoveryAcknowledgment error = %v", err)
	}
	items := s.items[plan.ID]
	for _, item := range items {
		if item.Status != string(store.RecoveryItemStatusRestored) {
			t.Fatalf("expected item restored, got %s", item.Status)
		}
	}
}

func TestReconcileRecoveryAcknowledgmentTimeout(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "target-1",
		Status:    string(store.RecoveryItemStatusAwaitingBeacon),
		UpdatedAt: time.Now().UTC().Add(-2 * DefaultRecoveryAcknowledgmentTimeout),
	})
	s.nodes["target-1"] = store.Node{ID: "target-1"}
	c := &Coordinator{store: s}

	err := c.ReconcileRecoveryAcknowledgment(context.Background())
	if err != nil {
		t.Fatalf("ReconcileRecoveryAcknowledgment error = %v", err)
	}
	items := s.items[plan.ID]
	for _, item := range items {
		if item.Status != string(store.RecoveryItemStatusFailed) {
			t.Fatalf("expected item failed after timeout, got %s", item.Status)
		}
	}
}

func TestReconcileRecoveryAcknowledgmentNilCoordinator(t *testing.T) {
	var nilCoord *Coordinator
	err := nilCoord.ReconcileRecoveryAcknowledgment(context.Background())
	if err == nil {
		t.Fatal("expected error from nil coordinator")
	}
}

func TestFinishBackupRestorePlanAwaitingBeaconNonTerminal(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1",
		Status: string(store.RecoveryItemStatusAwaitingBeacon),
	})
	c := &Coordinator{store: s}

	result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("finishBackupRestorePlan error = %v", err)
	}
	if result.Status == store.RecoveryPlanStatusRestored || result.Status == store.RecoveryPlanStatusCompleted {
		t.Fatalf("plan should not complete with awaiting_beacon item, got %v", result.Status)
	}
}

func TestFinishBackupRestorePlanHealthGatingNonTerminal(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1",
		Status: string(store.RecoveryItemStatusHealthGating),
	})
	c := &Coordinator{store: s}

	result, err := c.finishBackupRestorePlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("finishBackupRestorePlan error = %v", err)
	}
	if result.Status == store.RecoveryPlanStatusRestored || result.Status == store.RecoveryPlanStatusCompleted {
		t.Fatalf("plan should not complete with health_gating item, got %v", result.Status)
	}
}

func TestReconcileRecoveryAcknowledgmentHealthGatingToRestored(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "target-1",
		Status:    string(store.RecoveryItemStatusHealthGating),
		UpdatedAt: time.Now().UTC(),
	})
	c := &Coordinator{store: s}

	err := c.ReconcileRecoveryAcknowledgment(context.Background())
	if err != nil {
		t.Fatalf("ReconcileRecoveryAcknowledgment error = %v", err)
	}
	items := s.items[plan.ID]
	for _, item := range items {
		if item.Status != string(store.RecoveryItemStatusRestored) {
			t.Fatalf("expected health_gating item restored, got %s", item.Status)
		}
	}
}
