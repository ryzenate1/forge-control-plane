package placement

import (
	"context"
	"fmt"
	"math"
	"math/rand"
)

type Strategy string

const (
	StrategyLeastLoaded Strategy = "least-loaded"
	StrategyBinPack     Strategy = "bin-pack"
	StrategySpread      Strategy = "spread"
	StrategyRandom      Strategy = "random"
)

type Scorer interface {
	Name() string
	Score(ctx context.Context, candidate Candidate, request WorkloadRequest) (float64, []string, error)
}

type Candidate struct {
	NodeID          string
	RegionID        string
	TotalCPU        int
	TotalMemory     int
	TotalDisk       int
	AllocatedCPU    int
	AllocatedMemory int
	AllocatedDisk   int
	AvailableCPU    int
	AvailableMemory int
	AvailableDisk   int
	ServerCount     int
	Maintenance     bool
	Draining        bool
	Status          string
}

type WorkloadRequest struct {
	CPU           int
	MemoryMB      int
	DiskMB        int
	PreferredNode string
	RequiredNode  string
	RegionID      string
	Constraints   []Constraint
	ConstraintCtx ConstraintContext
}

type ScoreResult struct {
	NodeID  string
	Score   float64
	Reasons []string
}

func NewScorer(strategy Strategy) Scorer {
	switch strategy {
	case StrategyBinPack:
		return &BinPackScorer{}
	case StrategySpread:
		return &SpreadScorer{}
	case StrategyRandom:
		return NewRandomScorer()
	default:
		return &LeastLoadedScorer{}
	}
}

type LeastLoadedScorer struct{}

func (s *LeastLoadedScorer) Name() string { return string(StrategyLeastLoaded) }

func (s *LeastLoadedScorer) Score(_ context.Context, candidate Candidate, _ WorkloadRequest) (float64, []string, error) {
	score := float64(candidate.AvailableMemory)*1e9 + float64(candidate.AvailableCPU)*1e3 + float64(candidate.AvailableDisk)
	reasons := []string{
		fmt.Sprintf("available memory: %d MB", candidate.AvailableMemory),
		fmt.Sprintf("available CPU: %d shares", candidate.AvailableCPU),
		fmt.Sprintf("available disk: %d MB", candidate.AvailableDisk),
	}
	return score, reasons, nil
}

type BinPackScorer struct{}

func (s *BinPackScorer) Name() string { return string(StrategyBinPack) }

func (s *BinPackScorer) Score(_ context.Context, candidate Candidate, _ WorkloadRequest) (float64, []string, error) {
	var memUtil, cpuUtil, diskUtil float64
	if candidate.TotalMemory > 0 {
		memUtil = float64(candidate.AllocatedMemory) / float64(candidate.TotalMemory)
	}
	if candidate.TotalCPU > 0 {
		cpuUtil = float64(candidate.AllocatedCPU) / float64(candidate.TotalCPU)
	}
	if candidate.TotalDisk > 0 {
		diskUtil = float64(candidate.AllocatedDisk) / float64(candidate.TotalDisk)
	}
	score := (memUtil + cpuUtil + diskUtil) / 3.0
	reasons := []string{
		fmt.Sprintf("memory utilization: %.0f%%", math.Round(memUtil*100)),
		fmt.Sprintf("CPU utilization: %.0f%%", math.Round(cpuUtil*100)),
		fmt.Sprintf("disk utilization: %.0f%%", math.Round(diskUtil*100)),
	}
	return score, reasons, nil
}

type SpreadScorer struct{}

func (s *SpreadScorer) Name() string { return string(StrategySpread) }

func (s *SpreadScorer) Score(_ context.Context, candidate Candidate, _ WorkloadRequest) (float64, []string, error) {
	score := 1.0 / (1.0 + float64(candidate.ServerCount))
	reasons := []string{
		fmt.Sprintf("server count: %d", candidate.ServerCount),
		fmt.Sprintf("inverse weight: %.3f", score),
	}
	return score, reasons, nil
}

type RandomScorer struct {
	rng *rand.Rand
}

func NewRandomScorer() *RandomScorer {
	return &RandomScorer{
		rng: rand.New(rand.NewSource(42)),
	}
}

func (s *RandomScorer) Name() string { return string(StrategyRandom) }

func (s *RandomScorer) Score(_ context.Context, candidate Candidate, _ WorkloadRequest) (float64, []string, error) {
	score := s.rng.Float64()
	reasons := []string{
		fmt.Sprintf("random score: %.4f", score),
	}
	return score, reasons, nil
}
