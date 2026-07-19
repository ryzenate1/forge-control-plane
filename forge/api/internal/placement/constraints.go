package placement

import (
	"errors"
	"fmt"
	"slices"
)

type ConstraintType string

const (
	ConstraintAffinity     ConstraintType = "affinity"
	ConstraintAntiAffinity ConstraintType = "anti-affinity"
	ConstraintRegion       ConstraintType = "region"
	ConstraintNode         ConstraintType = "node"
	ConstraintLabel        ConstraintType = "label"
)

type Constraint struct {
	Type     ConstraintType
	Operator string
	Key      string
	Values   []string
	Required bool
}

type ConstraintContext struct {
	ServerNodeMap map[string]string
	NodeLabels    map[string]map[string]string
}

type ConstraintChecker struct{}

func NewConstraintChecker() *ConstraintChecker {
	return &ConstraintChecker{}
}

func (c *ConstraintChecker) CheckHard(candidate Candidate, constraints []Constraint, ctx ConstraintContext) error {
	for _, constraint := range constraints {
		if !constraint.Required {
			continue
		}
		if err := c.checkSingle(candidate, constraint, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (c *ConstraintChecker) CheckSoft(candidate Candidate, constraints []Constraint, ctx ConstraintContext) (float64, []string) {
	var bonus float64
	var reasons []string
	for _, constraint := range constraints {
		if constraint.Required {
			continue
		}
		err := c.checkSingle(candidate, constraint, ctx)
		if err == nil {
			bonus += 1e12
			reasons = append(reasons, fmt.Sprintf("soft constraint satisfied: %s %s", constraint.Type, constraint.Key))
		} else {
			bonus -= 1e10
			reasons = append(reasons, fmt.Sprintf("soft constraint not satisfied: %s %s", constraint.Type, constraint.Key))
		}
	}
	return bonus, reasons
}

func (c *ConstraintChecker) checkSingle(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	switch constraint.Type {
	case ConstraintAffinity:
		return c.checkAffinity(candidate, constraint, ctx)
	case ConstraintAntiAffinity:
		return c.checkAntiAffinity(candidate, constraint, ctx)
	case ConstraintRegion:
		return c.checkRegion(candidate, constraint)
	case ConstraintNode:
		return c.checkNode(candidate, constraint)
	case ConstraintLabel:
		return c.checkLabel(candidate, constraint, ctx)
	default:
		return fmt.Errorf("unknown constraint type: %s", constraint.Type)
	}
}

func (c *ConstraintChecker) checkAffinity(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	for _, serverID := range constraint.Values {
		nodeID, ok := ctx.ServerNodeMap[serverID]
		if !ok {
			return fmt.Errorf("affinity target server %s not found", serverID)
		}
		if nodeID == candidate.NodeID {
			return nil
		}
	}
	return fmt.Errorf("node %s does not satisfy affinity with servers %v", candidate.NodeID, constraint.Values)
}

func (c *ConstraintChecker) checkAntiAffinity(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	for _, serverID := range constraint.Values {
		nodeID, ok := ctx.ServerNodeMap[serverID]
		if !ok {
			continue
		}
		if nodeID == candidate.NodeID {
			return fmt.Errorf("node %s violates anti-affinity with server %s", candidate.NodeID, serverID)
		}
	}
	return nil
}

func (c *ConstraintChecker) checkRegion(candidate Candidate, constraint Constraint) error {
	switch constraint.Operator {
	case "in":
		if !slices.Contains(constraint.Values, candidate.RegionID) {
			return fmt.Errorf("node %s region %s not in %v", candidate.NodeID, candidate.RegionID, constraint.Values)
		}
	case "not-in":
		if slices.Contains(constraint.Values, candidate.RegionID) {
			return fmt.Errorf("node %s region %s is in excluded set %v", candidate.NodeID, candidate.RegionID, constraint.Values)
		}
	default:
		if !slices.Contains(constraint.Values, candidate.RegionID) {
			return fmt.Errorf("node %s region %s does not match %v", candidate.NodeID, candidate.RegionID, constraint.Values)
		}
	}
	return nil
}

func (c *ConstraintChecker) checkNode(candidate Candidate, constraint Constraint) error {
	switch constraint.Operator {
	case "in":
		if !slices.Contains(constraint.Values, candidate.NodeID) {
			return fmt.Errorf("node %s not in allowed set %v", candidate.NodeID, constraint.Values)
		}
	case "not-in":
		if slices.Contains(constraint.Values, candidate.NodeID) {
			return fmt.Errorf("node %s is in excluded set %v", candidate.NodeID, constraint.Values)
		}
	default:
		if !slices.Contains(constraint.Values, candidate.NodeID) {
			return errors.New("node " + candidate.NodeID + " does not match constraint")
		}
	}
	return nil
}

func (c *ConstraintChecker) checkLabel(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	labels, ok := ctx.NodeLabels[candidate.NodeID]
	if !ok {
		return fmt.Errorf("node %s has no labels", candidate.NodeID)
	}
	switch constraint.Operator {
	case "exists":
		if _, exists := labels[constraint.Key]; !exists {
			return fmt.Errorf("node %s missing label %s", candidate.NodeID, constraint.Key)
		}
	case "not-exists":
		if _, exists := labels[constraint.Key]; exists {
			return fmt.Errorf("node %s has excluded label %s", candidate.NodeID, constraint.Key)
		}
	case "in":
		val, exists := labels[constraint.Key]
		if !exists || !slices.Contains(constraint.Values, val) {
			return fmt.Errorf("node %s label %s value %s not in %v", candidate.NodeID, constraint.Key, val, constraint.Values)
		}
	case "not-in":
		val, exists := labels[constraint.Key]
		if exists && slices.Contains(constraint.Values, val) {
			return fmt.Errorf("node %s label %s value %s is in excluded set %v", candidate.NodeID, constraint.Key, val, constraint.Values)
		}
	default:
		val, exists := labels[constraint.Key]
		if !exists || !slices.Contains(constraint.Values, val) {
			return fmt.Errorf("node %s label %s does not satisfy constraint", candidate.NodeID, constraint.Key)
		}
	}
	return nil
}

func (c *ConstraintChecker) FilterByConstraints(candidates []Candidate, constraints []Constraint, ctx ConstraintContext) ([]Candidate, []string) {
	var filtered []Candidate
	var reasons []string
	for _, candidate := range candidates {
		err := c.CheckHard(candidate, constraints, ctx)
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("node %s excluded: %s", candidate.NodeID, err.Error()))
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered, reasons
}
