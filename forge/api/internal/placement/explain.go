package placement

import (
	"context"
)

type PlacementReport struct {
	Request          WorkloadRequest
	TotalCandidates  int
	FilteredOut      []FilterReason
	ScoredCandidates []ScoreResult
	Selected         *ScoreResult
}

type FilterReason struct {
	NodeID     string
	Constraint Constraint
	Reason     string
}

type ScoreBreakdown struct {
	NodeID              string
	BaseScore           float64
	SoftConstraintBonus float64
	Reasons             []string
}

func ExplainPlacement(ctx context.Context, engine *Engine, candidates []Candidate, req WorkloadRequest) (*PlacementReport, error) {
	report := &PlacementReport{
		Request:         req,
		TotalCandidates: len(candidates),
	}

	constraintCtx := req.ConstraintCtx
	var hardConstraints []Constraint
	for _, c := range req.Constraints {
		if c.Required {
			hardConstraints = append(hardConstraints, c)
		}
	}

	var viable []Candidate
	for _, candidate := range candidates {
		passed := true
		for _, constraint := range hardConstraints {
			if err := engine.checker.checkSingle(candidate, constraint, constraintCtx); err != nil {
				report.FilteredOut = append(report.FilteredOut, FilterReason{
					NodeID:     candidate.NodeID,
					Constraint: constraint,
					Reason:     err.Error(),
				})
				passed = false
				break
			}
		}
		if passed {
			viable = append(viable, candidate)
		}
	}

	for _, c := range viable {
		score, reasons, err := engine.scorer.Score(ctx, c, req)
		if err != nil {
			continue
		}
		bonus, bonusReasons := engine.checker.CheckSoft(c, req.Constraints, constraintCtx)
		totalScore := score + bonus
		allReasons := append(reasons, bonusReasons...)
		report.ScoredCandidates = append(report.ScoredCandidates, ScoreResult{
			NodeID:  c.NodeID,
			Score:   totalScore,
			Reasons: allReasons,
		})
	}

	if len(report.ScoredCandidates) > 0 {
		best := report.ScoredCandidates[0]
		for _, s := range report.ScoredCandidates[1:] {
			if s.Score > best.Score {
				best = s
			}
		}
		report.Selected = &best
	}

	return report, nil
}

func ExplainScores(ctx context.Context, scorer Scorer, checker *ConstraintChecker, candidates []Candidate, req WorkloadRequest) []ScoreBreakdown {
	var breakdowns []ScoreBreakdown
	for _, c := range candidates {
		score, reasons, err := scorer.Score(ctx, c, req)
		if err != nil {
			continue
		}
		bonus, bonusReasons := checker.CheckSoft(c, req.Constraints, req.ConstraintCtx)
		allReasons := append(reasons, bonusReasons...)
		breakdowns = append(breakdowns, ScoreBreakdown{
			NodeID:              c.NodeID,
			BaseScore:           score,
			SoftConstraintBonus: bonus,
			Reasons:             allReasons,
		})
	}
	return breakdowns
}
