package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplainPlacement_ProducesReport(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
		{NodeID: "node-3", RegionID: "us-east", AvailableMemory: 300},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"us-east"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{Constraints: constraints})
	require.NoError(t, err)

	assert.Equal(t, 3, report.TotalCandidates)
	assert.Len(t, report.FilteredOut, 1)
	assert.Equal(t, "node-2", report.FilteredOut[0].NodeID)
	assert.Len(t, report.ScoredCandidates, 2)
	require.NotNil(t, report.Selected)
	assert.Equal(t, "node-3", report.Selected.NodeID)
}

func TestExplainScores_ReturnsBreakdowns(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000},
		{NodeID: "node-2", AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000},
	}

	breakdowns := ExplainScores(context.Background(), &LeastLoadedScorer{}, NewConstraintChecker(), candidates, WorkloadRequest{})
	require.Len(t, breakdowns, 2)
	assert.Equal(t, "node-1", breakdowns[0].NodeID)
	assert.True(t, breakdowns[0].BaseScore > 0)
	assert.Equal(t, "node-2", breakdowns[1].NodeID)
	assert.True(t, breakdowns[1].BaseScore > breakdowns[0].BaseScore)
}

func TestExplainPlacement_WithAffinityConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
		{NodeID: "node-3", RegionID: "us-east", AvailableMemory: 300},
	}

	constraints := []Constraint{
		{Type: ConstraintAffinity, Required: true, Values: []string{"server-a"}},
	}

	ctx := ConstraintContext{
		ServerNodeMap: map[string]string{"server-a": "node-3"},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)

	assert.Equal(t, 3, report.TotalCandidates)
	assert.Len(t, report.FilteredOut, 2)
	assert.Len(t, report.ScoredCandidates, 1)
	require.NotNil(t, report.Selected)
	assert.Equal(t, "node-3", report.Selected.NodeID)
}

func TestExplainPlacement_WithAntiAffinityConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
		{NodeID: "node-3", RegionID: "us-east", AvailableMemory: 300},
	}

	constraints := []Constraint{
		{Type: ConstraintAntiAffinity, Required: true, Values: []string{"server-a"}},
	}

	ctx := ConstraintContext{
		ServerNodeMap: map[string]string{"server-a": "node-1"},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)

	assert.Equal(t, 3, report.TotalCandidates)
	assert.Len(t, report.FilteredOut, 1)
	assert.Equal(t, "node-1", report.FilteredOut[0].NodeID)
	assert.Len(t, report.ScoredCandidates, 2)
}

func TestExplainPlacement_WithLabelConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1"},
		{NodeID: "node-2"},
		{NodeID: "node-3"},
	}

	constraints := []Constraint{
		{Type: ConstraintLabel, Required: true, Operator: "in", Key: "tier", Values: []string{"gpu"}},
	}

	ctx := ConstraintContext{
		NodeLabels: map[string]map[string]string{
			"node-1": {"tier": "cpu"},
			"node-2": {"tier": "gpu"},
			"node-3": {"tier": "cpu"},
		},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)

	assert.Equal(t, 3, report.TotalCandidates)
	assert.Len(t, report.FilteredOut, 2)
	assert.Len(t, report.ScoredCandidates, 1)
	require.NotNil(t, report.Selected)
	assert.Equal(t, "node-2", report.Selected.NodeID)
}

func TestExplainScores_WithSoftConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000},
		{NodeID: "node-2", AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}},
	}

	req := WorkloadRequest{
		Constraints: constraints,
		ConstraintCtx: ConstraintContext{},
	}

	breakdowns := ExplainScores(context.Background(), &LeastLoadedScorer{}, NewConstraintChecker(), candidates, req)
	require.Len(t, breakdowns, 2)
	assert.Equal(t, "node-1", breakdowns[0].NodeID)
	assert.NotZero(t, breakdowns[0].BaseScore)
	assert.NotZero(t, breakdowns[0].SoftConstraintBonus)
	assert.Contains(t, breakdowns[0].Reasons, "soft constraint not satisfied: region ")
}

func TestExplainPlacement_NoViableCandidates(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east"},
		{NodeID: "node-2", RegionID: "us-west"},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"eu-west"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{Constraints: constraints})
	require.NoError(t, err)

	assert.Equal(t, 2, report.TotalCandidates)
	assert.Len(t, report.FilteredOut, 2)
	assert.Len(t, report.ScoredCandidates, 0)
	assert.Nil(t, report.Selected)
}
