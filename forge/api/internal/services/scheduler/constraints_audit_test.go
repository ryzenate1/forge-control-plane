package scheduler

import (
	"context"
	"testing"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

func regionPtr(id string) *string { return &id }

func TestConstraintSchedulerForbiddenExcludes(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: ConstraintForbidden, Key: "region", Operator: "eq", Value: "eu-west"})

	nodes := []store.Node{
		{ID: "n-eu", RegionID: regionPtr("eu-west")},
		{ID: "n-us", RegionID: regionPtr("us-east")},
	}
	filtered, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, nodes)
	if err != nil {
		t.Fatalf("EvaluateConstraints: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "n-us" {
		t.Fatalf("forbidden region should exclude n-eu, got %v", filtered)
	}
}

func TestConstraintSchedulerRequiredStillExcludes(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: ConstraintRequired, Key: "node_id", Operator: "eq", Value: "n-keep"})

	nodes := []store.Node{{ID: "n-keep"}, {ID: "n-drop"}}
	filtered, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, nodes)
	if err != nil {
		t.Fatalf("EvaluateConstraints: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "n-keep" {
		t.Fatalf("required node_id should keep only n-keep, got %v", filtered)
	}
}

func TestConstraintSchedulerPreferredNeverExcludes(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: ConstraintPreferred, Key: "region", Operator: "eq", Value: "nowhere"})

	nodes := []store.Node{{ID: "n-1"}, {ID: "n-2"}}
	filtered, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, nodes)
	if err != nil {
		t.Fatalf("EvaluateConstraints: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("preferred constraints must never exclude, got %v", filtered)
	}
}

func TestConstraintSchedulerRejectsUnknownOperator(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: ConstraintRequired, Key: "region", Operator: "frobnicates", Value: "x"})

	_, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, []store.Node{{ID: "n-1"}})
	if err == nil {
		t.Fatal("unknown operator must be rejected, not treated as satisfied")
	}
}

func TestConstraintSchedulerRejectsUnknownKey(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: ConstraintRequired, Key: "datacenter", Operator: "eq", Value: "x"})

	_, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, []store.Node{{ID: "n-1"}})
	if err == nil {
		t.Fatal("unknown key must be rejected, not treated as satisfied")
	}
}

func TestConstraintSchedulerRejectsUnknownType(t *testing.T) {
	cs := NewConstraintScheduler(nil)
	cs.AddConstraint(Constraint{Type: "banished", Key: "region", Operator: "eq", Value: "x"})

	_, err := cs.EvaluateConstraints(context.Background(), domain.PlacementRequest{}, []store.Node{{ID: "n-1"}})
	if err == nil {
		t.Fatal("unknown constraint type must be rejected")
	}
}

func TestToPlacementConstraintsForbiddenInverts(t *testing.T) {
	out := toPlacementConstraints([]domain.PlacementConstraint{
		{Type: domain.PlacementConstraintForbidden, Key: "region", Operator: "eq", Value: "eu-west"},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 converted constraint, got %d", len(out))
	}
	if !out[0].Required {
		t.Fatal("forbidden must convert to a required exclusion")
	}
	if out[0].Operator != "not-in" {
		t.Fatalf("forbidden eq should invert to not-in, got %q", out[0].Operator)
	}
}
