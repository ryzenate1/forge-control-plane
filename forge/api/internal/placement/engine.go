package placement

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Logger interface {
	Log(ctx context.Context, msg string, args ...any)
}

// Engine is safe for concurrent use: scorer, checker and logger are fixed at
// construction and scoring itself touches no shared state. A scorer that is not
// safe for concurrent use guards its own mutable fields (see RandomScorer).
type Engine struct {
	scorer  Scorer
	checker *ConstraintChecker
	logger  Logger
}

func NewEngine(scorer Scorer, checker *ConstraintChecker) *Engine {
	return &Engine{
		scorer:  scorer,
		checker: checker,
	}
}

func (e *Engine) WithLogger(l Logger) *Engine {
	e.logger = l
	return e
}

func (e *Engine) Scorer() Scorer {
	return e.scorer
}

func (e *Engine) Place(ctx context.Context, candidates []Candidate, req WorkloadRequest) (ScoreResult, error) {
	results, _, err := e.evaluate(ctx, candidates, req)
	if err != nil {
		return ScoreResult{}, err
	}

	best := results[0]
	tied := 1
	for _, r := range results[1:] {
		switch {
		case r.Score > best.Score:
			best = r
			tied = 1
		case r.Score == best.Score:
			tied++
		}
	}

	e.log(ctx, "placement selected", "node", best.NodeID, "score", best.Score)
	if tied > 1 {
		// Equal scores are resolved by candidate order, which is an arbitrary
		// tie-break rather than a decision — say so while it is still visible.
		e.log(ctx, "placement tie broken by candidate order", "node", best.NodeID, "score", best.Score, "tied candidates", tied)
	}

	return best, nil
}

func (e *Engine) PlaceAll(ctx context.Context, candidates []Candidate, req WorkloadRequest) ([]ScoreResult, error) {
	results, _, err := e.evaluate(ctx, candidates, req)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// evaluate filters, scores and soft-weights every candidate. A candidate that
// cannot be scored is recorded in the returned failures: excluded is the safe
// direction, but dropped silently it leaves an operator with a short ranking and
// no way to tell a full node from a broken snapshot.
func (e *Engine) evaluate(ctx context.Context, candidates []Candidate, req WorkloadRequest) ([]ScoreResult, []ScoreFailure, error) {
	filtered, reasons := e.checker.FilterByConstraints(candidates, req.Constraints, req.ConstraintCtx)
	for _, r := range reasons {
		e.log(ctx, "filtered candidate", "reason", r)
	}
	if len(filtered) == 0 {
		return nil, nil, errors.New("no viable candidates after constraint filtering")
	}

	results := make([]ScoreResult, 0, len(filtered))
	var failures []ScoreFailure
	for _, c := range filtered {
		if err := ctx.Err(); err != nil {
			return nil, failures, fmt.Errorf("placement cancelled: %w", err)
		}
		score, scoreReasons, err := e.scorer.Score(ctx, c, req)
		if err != nil {
			failures = append(failures, ScoreFailure{NodeID: c.NodeID, Reason: err.Error()})
			e.log(ctx, "candidate failed scoring", "node", c.NodeID, "reason", err.Error())
			continue
		}
		bonus, bonusReasons := e.checker.CheckSoft(c, req.Constraints, req.ConstraintCtx)
		allReasons := make([]string, 0, len(scoreReasons)+len(bonusReasons))
		allReasons = append(allReasons, scoreReasons...)
		allReasons = append(allReasons, bonusReasons...)
		results = append(results, ScoreResult{NodeID: c.NodeID, Score: score + bonus, Reasons: allReasons, StorageLocality: c.StorageLocality})
	}

	if len(results) == 0 {
		if len(failures) == 0 {
			return nil, nil, errors.New("no viable candidates after scoring")
		}
		return nil, failures, fmt.Errorf("no viable candidates after scoring: %s", describeScoreFailures(failures))
	}

	return results, failures, nil
}

func (e *Engine) log(ctx context.Context, msg string, args ...any) {
	if e.logger == nil {
		return
	}
	e.logger.Log(ctx, msg, args...)
}

const (
	maxFailureReasons = 5
	maxFailureNodes   = 5
)

// describeScoreFailures groups repeated reasons so a fleet-wide scorer error
// reads as one clause while a per-node shortfall stays attributed to its node.
func describeScoreFailures(failures []ScoreFailure) string {
	order := make([]string, 0, len(failures))
	nodesByReason := make(map[string][]string, len(failures))
	for _, f := range failures {
		if _, seen := nodesByReason[f.Reason]; !seen {
			order = append(order, f.Reason)
		}
		nodesByReason[f.Reason] = append(nodesByReason[f.Reason], f.NodeID)
	}

	parts := make([]string, 0, len(order))
	for i, reason := range order {
		if i >= maxFailureReasons {
			parts = append(parts, fmt.Sprintf("%d further distinct scoring failures", len(order)-i))
			break
		}
		nodes := nodesByReason[reason]
		shown, suffix := nodes, ""
		if len(nodes) > maxFailureNodes {
			shown, suffix = nodes[:maxFailureNodes], fmt.Sprintf(" and %d more", len(nodes)-maxFailureNodes)
		}
		parts = append(parts, fmt.Sprintf("%s (%s%s)", reason, strings.Join(shown, ", "), suffix))
	}
	return strings.Join(parts, "; ")
}
