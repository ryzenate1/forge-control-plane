package recovery

import (
	"context"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestPlanStatus(t *testing.T) {
	s := newMockStore()
	s.nodes["node-1"] = store.Node{ID: "node-1", HeartbeatState: "offline", ActualState: "offline"}
	c := &Coordinator{store: s}

	t.Run("returns status view for existing plan", func(t *testing.T) {
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", Status: string(store.RecoveryItemStatusPlanned),
		})

		view, err := c.PlanStatus(context.Background(), plan.ID)
		if err != nil {
			t.Fatalf("PlanStatus error: %v", err)
		}
		if view.ID != plan.ID {
			t.Fatalf("plan ID mismatch: %q vs %q", view.ID, plan.ID)
		}
		if view.ItemCount != 1 {
			t.Fatalf("expected 1 item, got %d", view.ItemCount)
		}
	})

	t.Run("nil coordinator returns error", func(t *testing.T) {
		var nilCoord *Coordinator
		_, err := nilCoord.PlanStatus(context.Background(), "plan-1")
		if err == nil {
			t.Fatal("expected error from nil coordinator")
		}
	})
}

func TestListPlanStatuses(t *testing.T) {
	s := newMockStore()
	c := &Coordinator{store: s}

	t.Run("lists all plans", func(t *testing.T) {
		s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryPlan(context.Background(), "node-2", "test")

		views, err := c.ListPlanStatuses(context.Background())
		if err != nil {
			t.Fatalf("ListPlanStatuses error: %v", err)
		}
		if len(views) != 2 {
			t.Fatalf("expected 2 plans, got %d", len(views))
		}
	})
}

func TestConfirmDestructiveRecovery(t *testing.T) {
	s := newMockStore()
	c := &Coordinator{store: s}

	t.Run("requires force for destructive items", func(t *testing.T) {
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
			Status: string(store.RecoveryItemStatusPlanned),
		})
		s.UpdateRecoveryPlanStatus(context.Background(), plan.ID, store.RecoveryPlanStatusPlanned, "planned")

		_, err := c.ConfirmDestructiveRecovery(context.Background(), ConfirmRecoveryRequest{PlanID: plan.ID})
		if err == nil {
			t.Fatal("expected error for destructive recovery without force")
		}
	})

	t.Run("proceeds with force", func(t *testing.T) {
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
			ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
			Status: string(store.RecoveryItemStatusPlanned),
		})
		s.UpdateRecoveryPlanStatus(context.Background(), plan.ID, store.RecoveryPlanStatusPlanned, "planned")

		_, err := c.ConfirmDestructiveRecovery(context.Background(), ConfirmRecoveryRequest{PlanID: plan.ID, Force: true})
		if err != nil {
			t.Fatalf("ConfirmDestructiveRecovery error: %v", err)
		}
	})
}

func TestRecoverFromUnavailable(t *testing.T) {
	s := newMockStore()
	s.nodes["node-1"] = store.Node{ID: "node-1", HeartbeatState: "offline", ActualState: "offline"}
	c := &Coordinator{store: s}

	t.Run("creates recovery plan for offline node", func(t *testing.T) {
		plan, err := c.RecoverFromUnavailable(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("RecoverFromUnavailable error: %v", err)
		}
		if plan.ID == "" {
			t.Fatal("expected non-empty plan ID")
		}
	})

	t.Run("nil coordinator returns error", func(t *testing.T) {
		var nilCoord *Coordinator
		_, err := nilCoord.RecoverFromUnavailable(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil coordinator")
		}
	})
}

func TestPostRecoveryAction(t *testing.T) {
	s := newMockStore()
	c := &Coordinator{store: s}

	t.Run("accepts post-recovery action on restored plan", func(t *testing.T) {
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
		s.UpdateRecoveryPlanStatus(context.Background(), plan.ID, store.RecoveryPlanStatusRestored, "restored")

		_, err := c.PostRecoveryAction(context.Background(), plan.ID, PostRecoveryActionStartWorkloads)
		if err != nil {
			t.Fatalf("PostRecoveryAction error: %v", err)
		}
	})

	t.Run("rejects action on non-terminal plan", func(t *testing.T) {
		plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")

		_, err := c.PostRecoveryAction(context.Background(), plan.ID, PostRecoveryActionStartWorkloads)
		if err == nil {
			t.Fatal("expected error on non-terminal plan")
		}
	})
}

func TestToStatusView(t *testing.T) {
	s := newMockStore()
	plan, _ := s.CreateRecoveryPlan(context.Background(), "node-1", "test")
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
		Status: string(store.RecoveryItemStatusPlanned),
	})
	s.CreateRecoveryItem(context.Background(), plan.ID, store.RecoveryItem{
		ServerID: "srv-2", SourceNodeID: "node-1",
		Status: string(store.RecoveryItemStatusSkipped),
		Reason: "no backup",
	})
	plan, _ = s.GetRecoveryPlan(context.Background(), plan.ID)

	c := &Coordinator{store: s}
	view := c.toStatusView(context.Background(), plan)

	if view.ItemCount != 2 {
		t.Fatalf("expected 2 items, got %d", view.ItemCount)
	}
	if !view.NeedsConfirm {
		t.Fatal("expected needsConfirm=true with destructive items")
	}
	if !view.Items[0].Destructive {
		t.Fatal("expected first item to be destructive")
	}
	if view.Items[1].HasBackup {
		t.Fatal("expected second item to have no backup")
	}
}
