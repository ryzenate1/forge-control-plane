package scheduler

import (
	"context"
	"fmt"
	"strings"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

type ConstraintType string

const (
	ConstraintRequired  ConstraintType = "required"
	ConstraintPreferred ConstraintType = "preferred"
	ConstraintForbidden ConstraintType = "forbidden"
)

type Constraint struct {
	Type     ConstraintType `json:"type"`
	Key      string         `json:"key"`
	Operator string         `json:"operator"`
	Value    string         `json:"value"`
}

type ConstraintScheduler struct {
	store       constraintStore
	constraints []Constraint
}

type constraintStore interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	NodeCapacitySnapshot(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error)
}

func NewConstraintScheduler(store constraintStore) *ConstraintScheduler {
	return &ConstraintScheduler{
		store:       store,
		constraints: make([]Constraint, 0),
	}
}

func (s *ConstraintScheduler) AddConstraint(c Constraint) {
	s.constraints = append(s.constraints, c)
}

func (s *ConstraintScheduler) RemoveConstraint(index int) {
	if index >= 0 && index < len(s.constraints) {
		s.constraints = append(s.constraints[:index], s.constraints[index+1:]...)
	}
}

func (s *ConstraintScheduler) SetConstraints(constraints []Constraint) {
	s.constraints = constraints
}

func (s *ConstraintScheduler) GetConstraints() []Constraint {
	return s.constraints
}

func (s *ConstraintScheduler) EvaluateConstraints(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
	if len(s.constraints) == 0 {
		return nodes, nil
	}

	var result []store.Node
	for _, node := range nodes {
		if s.meetsAllConstraints(ctx, req, node) {
			result = append(result, node)
		}
	}
	return result, nil
}

func (s *ConstraintScheduler) meetsAllConstraints(ctx context.Context, req domain.PlacementRequest, node store.Node) bool {
	for _, c := range s.constraints {
		if !s.evaluateConstraint(ctx, c, req, node) {
			if c.Type == ConstraintRequired {
				return false
			}
		}
	}
	return true
}

func (s *ConstraintScheduler) evaluateConstraint(ctx context.Context, c Constraint, req domain.PlacementRequest, node store.Node) bool {
	value := s.getConstraintValue(ctx, c.Key, node)
	switch c.Operator {
	case "eq":
		return value == c.Value
	case "neq":
		return value != c.Value
	case "in":
		parts := strings.Split(c.Value, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == value {
				return true
			}
		}
		return false
	case "notin":
		parts := strings.Split(c.Value, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == value {
				return false
			}
		}
		return true
	case "exists":
		return value != ""
	default:
		return true
	}
}

func (s *ConstraintScheduler) getConstraintValue(ctx context.Context, key string, node store.Node) string {
	switch key {
	case "region":
		if node.RegionID != nil {
			return *node.RegionID
		}
		return ""
	case "node_id":
		return node.ID
	case "name":
		return node.Name
	default:
		return ""
	}
}

func (s *ConstraintScheduler) String() string {
	return fmt.Sprintf("ConstraintScheduler{constraints=%v}", s.constraints)
}
