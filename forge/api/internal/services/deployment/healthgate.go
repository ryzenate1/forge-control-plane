package deployment

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (s *Service) CheckHealth(ctx context.Context, deployment *Deployment) (*HealthCheckResult, error) {
	if deployment.HealthCheckPath == "" || deployment.HealthCheckPort == 0 {
		return &HealthCheckResult{Passed: true}, nil
	}

	host := deployment.HealthCheckHost
	if host == "" {
		host = "localhost"
	}
	target := fmt.Sprintf("http://%s:%d%s", host, deployment.HealthCheckPort, deployment.HealthCheckPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return &HealthCheckResult{Passed: false, Error: err.Error()}, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthCheckResult{Passed: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	passed := resp.StatusCode >= 200 && resp.StatusCode < 400

	return &HealthCheckResult{
		Passed: passed,
		Status: resp.StatusCode,
		Body:   string(body),
	}, nil
}

func (s *Service) WaitForHealthGate(ctx context.Context, deployment *Deployment, stepID string) error {
	if !deployment.HealthGateEnabled {
		return nil
	}

	threshold := deployment.HealthGateThreshold
	if threshold <= 0 {
		threshold = DefaultHealthGateThreshold
	}

	interval := deployment.HealthGateIntervalMs
	if interval <= 0 {
		interval = DefaultHealthGateIntervalMs
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()

	timeout := time.Duration(deployment.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	consecutiveSuccesses := 0
	for {
		select {
		case <-ctx.Done():
			_ = s.markStepFailed(context.Background(), stepID, fmt.Sprintf("health gate timed out after %d consecutive failures", threshold))
			return fmt.Errorf("health gate timed out after %d consecutive failures: %w", threshold, ErrHealthCheckFailed)
		case <-ticker.C:
			result, err := s.CheckHealth(ctx, deployment)
			if err != nil {
				return err
			}
			if result.Passed {
				consecutiveSuccesses++
				if consecutiveSuccesses >= threshold {
					return nil
				}
			} else {
				consecutiveSuccesses = 0
			}
		}
	}
}
