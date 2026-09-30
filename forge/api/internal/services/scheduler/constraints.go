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
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("constraint evaluation cancelled: %w", err)
		}
		ok, err := s.meetsAllConstraints(ctx, req, node)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, node)
		}
	}
	return result, nil
}

// meetsAllConstraints enforces every configured constraint: a required
// constraint the node does not satisfy excludes it, and a forbidden
// constraint the node does satisfy excludes it. Preferred constraints never
// exclude; they are scoring hints, not filters.
func (s *ConstraintScheduler) meetsAllConstraints(ctx context.Context, req domain.PlacementRequest, node store.Node) (bool, error) {
	for _, c := range s.constraints {
		matched, err := s.evaluateConstraint(ctx, c, req, node)
		if err != nil {
			return false, err
		}
		switch c.Type {
		case ConstraintRequired:
			if !matched {
				return false, nil
			}
		case ConstraintForbidden:
			if matched {
				return false, nil
			}
		case ConstraintPreferred:
			continue
		default:
			return false, fmt.Errorf("unknown constraint type %q", c.Type)
		}
	}
	return true, nil
}

// evaluateConstraint reports whether the node matches the constraint. An
// unknown operator or key is an error, never a pass: silently treating an
// unrecognized constraint as satisfied would admit nodes the operator meant
// to exclude.
func (s *ConstraintScheduler) evaluateConstraint(ctx context.Context, c Constraint, req domain.PlacementRequest, node store.Node) (bool, error) {
	value, err := s.getConstraintValue(ctx, c.Key, node)
	if err != nil {
		return false, err
	}
	switch c.Operator {
	case "eq":
		return value == c.Value, nil
	case "neq":
		return value != c.Value, nil
	case "in":
		parts := strings.Split(c.Value, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == value {
				return true, nil
			}
		}
		return false, nil
	case "notin":
		parts := strings.Split(c.Value, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == value {
				return false, nil
			}
		}
		return true, nil
	case "exists":
		return value != "", nil
	default:
		return false, fmt.Errorf("unknown constraint operator %q", c.Operator)
	}
}

func (s *ConstraintScheduler) getConstraintValue(ctx context.Context, key string, node store.Node) (string, error) {
	switch key {
	case "region":
		if node.RegionID != nil {
			return *node.RegionID, nil
		}
		return "", nil
	case "node_id":
		return node.ID, nil
	case "name":
		return node.Name, nil
	default:
		return "", fmt.Errorf("unknown constraint key %q", key)
	}
}

func (s *ConstraintScheduler) String() string {
	return fmt.Sprintf("ConstraintScheduler{constraints=%v}", s.constraints)
}
