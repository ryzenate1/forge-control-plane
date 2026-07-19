package health

import (
	"context"
	"sync"
	"time"
)

type Service struct {
	mu         sync.RWMutex
	checks     []Check
	started    time.Time
	version    string
	history    map[string][]CheckResult
	maxHistory int
}

func NewService(version string) *Service {
	return &Service{
		checks:     make([]Check, 0),
		started:    time.Now(),
		version:    version,
		history:    make(map[string][]CheckResult),
		maxHistory: 10,
	}
}

func (s *Service) AddCheck(check Check) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks = append(s.checks, check)
}

func (s *Service) RunAll(ctx context.Context) HealthReport {
	s.mu.RLock()
	checks := make([]Check, len(s.checks))
	copy(checks, s.checks)
	s.mu.RUnlock()

	results := make([]CheckResult, 0, len(checks))
	allOK := true
	aggregateStatus := StatusOK

	for _, check := range checks {
		result := check.Run(ctx)
		if critical, ok := check.(CriticalCheck); ok {
			result.Critical = critical.Critical()
		}

		s.mu.Lock()
		name := check.Name()
		s.history[name] = append(s.history[name], result)
		if len(s.history[name]) > s.maxHistory {
			s.history[name] = s.history[name][len(s.history[name])-s.maxHistory:]
		}
		s.mu.Unlock()

		switch result.Status {
		case StatusFailed:
			allOK = false
			aggregateStatus = StatusFailed
		case StatusWarning:
			if aggregateStatus == StatusOK {
				aggregateStatus = StatusWarning
			}
		}

		results = append(results, result)
	}

	return HealthReport{
		Status:    aggregateStatus,
		OK:        allOK,
		Service:   "api",
		Version:   s.version,
		Uptime:    time.Since(s.started).String(),
		Checks:    results,
		CheckedAt: time.Now(),
	}
}

func (s *Service) RunCheck(ctx context.Context, name string) *CheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, check := range s.checks {
		if check.Name() == name {
			result := check.Run(ctx)
			if critical, ok := check.(CriticalCheck); ok {
				result.Critical = critical.Critical()
			}
			return &result
		}
	}
	return nil
}

func (s *Service) GetCheckHistory(name string) []CheckResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.history[name]
	result := make([]CheckResult, len(history))
	copy(result, history)
	return result
}

func (s *Service) GetStatusSummary(ctx context.Context) map[string]string {
	report := s.RunAll(ctx)
	summary := make(map[string]string)
	for _, check := range report.Checks {
		summary[check.Name] = string(check.Status)
	}
	return summary
}
