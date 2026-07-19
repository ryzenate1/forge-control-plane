package placement

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_Place_SelectsHighestScored(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000},
		{NodeID: "node-2", AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000},
		{NodeID: "node-3", AvailableMemory: 50, AvailableCPU: 5, AvailableDisk: 500},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{})
	require.NoError(t, err)
	assert.Equal(t, "node-2", result.NodeID)
}

func TestEngine_Place_ReturnsErrorWhenNoCandidatesMatch(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east"},
		{NodeID: "node-2", RegionID: "us-west"},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"eu-west"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	_, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	assert.Error(t, err)
}

func TestEngine_PlaceAll_ReturnsSortedResults(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000},
		{NodeID: "node-2", AvailableMemory: 300, AvailableCPU: 30, AvailableDisk: 3000},
		{NodeID: "node-3", AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	results, err := engine.PlaceAll(context.Background(), candidates, WorkloadRequest{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "node-2", results[0].NodeID)
	assert.Equal(t, "node-3", results[1].NodeID)
	assert.Equal(t, "node-1", results[2].NodeID)
}

func TestEngine_Place_WithHardConstraints(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
		{NodeID: "node-3", RegionID: "us-east", AvailableMemory: 300},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"us-east"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	require.NoError(t, err)
	assert.Equal(t, "node-3", result.NodeID)
}

func TestEngine_PlaceAll_WithEmptyCandidates(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	_, err := engine.PlaceAll(context.Background(), []Candidate{}, WorkloadRequest{})
	assert.Error(t, err)
}

func TestEngine_PlaceAll_WithHardConstraints(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
		{NodeID: "node-3", RegionID: "us-east", AvailableMemory: 300},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"us-east"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	results, err := engine.PlaceAll(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "node-3", results[0].NodeID)
	assert.Equal(t, "node-1", results[1].NodeID)
}

func TestEngine_Place_WithAffinityConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100},
		{NodeID: "node-2", AvailableMemory: 200},
	}

	constraints := []Constraint{
		{Type: ConstraintAffinity, Required: true, Values: []string{"server-a"}},
	}

	ctx := ConstraintContext{
		ServerNodeMap: map[string]string{"server-a": "node-2"},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)
	assert.Equal(t, "node-2", result.NodeID)
}

func TestEngine_Place_WithAntiAffinityConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100},
		{NodeID: "node-2", AvailableMemory: 200},
	}

	constraints := []Constraint{
		{Type: ConstraintAntiAffinity, Required: true, Values: []string{"server-a"}},
	}

	ctx := ConstraintContext{
		ServerNodeMap: map[string]string{"server-a": "node-1"},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)
	assert.Equal(t, "node-2", result.NodeID)
}

func TestEngine_Place_WithLabelConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", AvailableMemory: 100},
		{NodeID: "node-2", AvailableMemory: 200},
	}

	constraints := []Constraint{
		{Type: ConstraintLabel, Required: true, Operator: "in", Key: "tier", Values: []string{"gpu"}},
	}

	ctx := ConstraintContext{
		NodeLabels: map[string]map[string]string{
			"node-1": {"tier": "cpu"},
			"node-2": {"tier": "gpu"},
		},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints, ConstraintCtx: ctx})
	require.NoError(t, err)
	assert.Equal(t, "node-2", result.NodeID)
}

func TestEngine_Place_WithSoftConstraint(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", AvailableMemory: 100},
		{NodeID: "node-2", RegionID: "us-west", AvailableMemory: 200},
	}

	constraints := []Constraint{
		{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	require.NoError(t, err)
	assert.Equal(t, "node-1", result.NodeID)
}
