package placement

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Operator spellings the checker implements. "notin"/"notexists" are the
// scheduler's own vocabulary (services/scheduler/constraints.go) and never
// reach this package: normalizeConstraintOperator translates them to the
// spellings below before the constraint is handed to the engine.
const (
	opIn          = "in"
	opNotIn       = "not-in"
	opExists      = "exists"
	opNotExists   = "not-exists"
	opUnspecified = ""
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
	var failures []string
	for _, constraint := range constraints {
		if !constraint.Required {
			continue
		}
		if err := c.checkSingle(candidate, constraint, ctx); err != nil {
			failures = append(failures, err.Error())
		}
	}
	switch len(failures) {
	case 0:
		return nil
	case 1:
		return errors.New(failures[0])
	default:
		// Report every unmet hard constraint, not just the first: an operator
		// asking why the fleet was rejected learns about the second reason only
		// after fixing the first.
		return fmt.Errorf("node %s failed %d required constraints: %s",
			candidate.NodeID, len(failures), strings.Join(failures, "; "))
	}
}

func (c *ConstraintChecker) CheckSoft(candidate Candidate, constraints []Constraint, ctx ConstraintContext) (float64, []string) {
	var reasons []string
	var evaluated, satisfied int
	for _, constraint := range constraints {
		if constraint.Required {
			continue
		}
		err := c.checkSingle(candidate, constraint, ctx)
		evaluated++
		if err == nil {
			satisfied++
			reasons = append(reasons, fmt.Sprintf("soft constraint satisfied: %s %s", constraint.Type, constraint.Key))
		} else {
			reasons = append(reasons, fmt.Sprintf("soft constraint not satisfied: %s %s", constraint.Type, constraint.Key))
		}
	}
	if evaluated == 0 {
		return 0, reasons
	}
	missed := evaluated - satisfied
	if !placementV2() {
		return float64(satisfied)*legacySoftWeight - float64(missed)*legacySoftPenalty, reasons
	}
	// Averaged rather than summed: ten copies of the same preference are still
	// one preference. Summing let the number of constraints, rather than their
	// strength, decide the placement.
	return (float64(satisfied)/float64(evaluated))*kSoftWeight -
		(float64(missed)/float64(evaluated))*kSoftPenalty, reasons
}

func (c *ConstraintChecker) checkSingle(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	if err := validateConstraint(constraint); err != nil {
		return err
	}
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

// operatorSpelling canonicalises what the caller wrote; effectiveOperator
// names the behaviour actually applied so errors read as the operator the
// checker ran, not as an empty string.
func operatorSpelling(op string) string {
	return strings.ToLower(strings.TrimSpace(op))
}

func effectiveOperator(op string) string {
	if op == opUnspecified {
		return opIn
	}
	return op
}

// validateConstraint rejects a constraint the checker cannot honestly
// evaluate. An unknown operator used to fall through and behave like "in", so
// a half-typed "notin" became a positive match, and an empty value list left a
// negative constraint excluding nothing — every node then read as compliant
// with a rule nobody finished writing.
func validateConstraint(constraint Constraint) error {
	op := operatorSpelling(constraint.Operator)
	switch constraint.Type {
	case ConstraintAffinity, ConstraintAntiAffinity:
		if op != opUnspecified && op != opIn {
			return fmt.Errorf("%s constraint does not implement operator %q (the type carries the direction)", constraint.Type, constraint.Operator)
		}
		if len(constraint.Values) == 0 {
			return fmt.Errorf("%s constraint requires at least one target server", constraint.Type)
		}
		return nil
	case ConstraintRegion, ConstraintNode:
		switch op {
		case opUnspecified, opIn, opNotIn:
		default:
			return fmt.Errorf("%s constraint does not implement operator %q (supported: in, not-in)", constraint.Type, constraint.Operator)
		}
		if len(constraint.Values) == 0 {
			return fmt.Errorf("%s constraint operator %q requires at least one value", constraint.Type, effectiveOperator(op))
		}
		return nil
	case ConstraintLabel:
		if strings.TrimSpace(constraint.Key) == "" {
			return errors.New("label constraint requires a key")
		}
		switch op {
		case opUnspecified, opIn, opNotIn, opExists, opNotExists:
		default:
			return fmt.Errorf("label constraint does not implement operator %q (supported: in, not-in, exists, not-exists)", constraint.Operator)
		}
		if op != opExists && op != opNotExists && len(constraint.Values) == 0 {
			return fmt.Errorf("label constraint operator %q requires at least one value", effectiveOperator(op))
		}
		return nil
	default:
		return fmt.Errorf("unknown constraint type: %s", constraint.Type)
	}
}

func (c *ConstraintChecker) checkAffinity(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	if len(ctx.ServerNodeMap) == 0 {
		// An absent map is not evidence that the target runs nowhere; it is the
		// checker being handed nothing to read.
		return fmt.Errorf("affinity with servers %v cannot be checked: no server-to-node map in the placement context", constraint.Values)
	}
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
	if len(ctx.ServerNodeMap) == 0 {
		// Separation cannot be promised against a map that was never built, so
		// every candidate is rejected and the reason says why — rather than the
		// constraint quietly excluding nothing.
		return fmt.Errorf("anti-affinity with servers %v cannot be checked: no server-to-node map in the placement context", constraint.Values)
	}
	for _, serverID := range constraint.Values {
		nodeID, ok := ctx.ServerNodeMap[serverID]
		if !ok {
			return fmt.Errorf("anti-affinity target server %s not found", serverID)
		}
		if nodeID == candidate.NodeID {
			return fmt.Errorf("node %s violates anti-affinity with server %s", candidate.NodeID, serverID)
		}
	}
	return nil
}

func (c *ConstraintChecker) checkRegion(candidate Candidate, constraint Constraint) error {
	switch effectiveOperator(operatorSpelling(constraint.Operator)) {
	case opIn:
		if !slices.Contains(constraint.Values, candidate.RegionID) {
			return fmt.Errorf("node %s region %s not in %v", candidate.NodeID, candidate.RegionID, constraint.Values)
		}
	case opNotIn:
		if slices.Contains(constraint.Values, candidate.RegionID) {
			return fmt.Errorf("node %s region %s is in excluded set %v", candidate.NodeID, candidate.RegionID, constraint.Values)
		}
	default:
		return fmt.Errorf("region constraint does not implement operator %q", constraint.Operator)
	}
	return nil
}

func (c *ConstraintChecker) checkNode(candidate Candidate, constraint Constraint) error {
	switch effectiveOperator(operatorSpelling(constraint.Operator)) {
	case opIn:
		if !slices.Contains(constraint.Values, candidate.NodeID) {
			return fmt.Errorf("node %s not in allowed set %v", candidate.NodeID, constraint.Values)
		}
	case opNotIn:
		if slices.Contains(constraint.Values, candidate.NodeID) {
			return fmt.Errorf("node %s is in excluded set %v", candidate.NodeID, constraint.Values)
		}
	default:
		return fmt.Errorf("node constraint does not implement operator %q", constraint.Operator)
	}
	return nil
}

func (c *ConstraintChecker) checkLabel(candidate Candidate, constraint Constraint, ctx ConstraintContext) error {
	op := effectiveOperator(operatorSpelling(constraint.Operator))
	// A node with no entry, or no labels at all, is read as an empty label set:
	// there is nothing to match, so "not-exists"/"not-in" are satisfied while
	// "exists"/"in" fail. Rejecting an absent label on a negative operator
	// turned "avoid nodes labelled X" into "reject every unlabelled node".
	labels, indexed := ctx.NodeLabels[candidate.NodeID]
	value, present := labels[constraint.Key]

	switch op {
	case opExists:
		if !present {
			return fmt.Errorf("node %s missing label %s (%s)", candidate.NodeID, constraint.Key, labelAvailability(indexed, ctx))
		}
	case opNotExists:
		if present {
			return fmt.Errorf("node %s has excluded label %s", candidate.NodeID, constraint.Key)
		}
	case opIn:
		if !present || !slices.Contains(constraint.Values, value) {
			return fmt.Errorf("node %s label %s value %q not in %v (%s)", candidate.NodeID, constraint.Key, value, constraint.Values, labelAvailability(indexed, ctx))
		}
	case opNotIn:
		if present && slices.Contains(constraint.Values, value) {
			return fmt.Errorf("node %s label %s value %s is in excluded set %v", candidate.NodeID, constraint.Key, value, constraint.Values)
		}
	default:
		return fmt.Errorf("label constraint does not implement operator %q", constraint.Operator)
	}
	return nil
}

// labelAvailability separates "this node has that label set and the key is not
// in it" from "nobody built the label index", so a missing index is not read as
// a fact about the node.
func labelAvailability(indexed bool, ctx ConstraintContext) string {
	if len(ctx.NodeLabels) == 0 {
		return "the placement context carries no label index"
	}
	if !indexed {
		return "node has no entry in the label index"
	}
	return "node does not carry the key"
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
