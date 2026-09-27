package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (s *Service) CheckHealth(ctx context.Context, deployment *Deployment) (*HealthCheckResult, error) {
	if deployment.HealthCheckPath == "" || deployment.HealthCheckPort == 0 {
		return &HealthCheckResult{Passed: true}, nil
	}

	host := deployment.HealthCheckHost
	if host == "" {
		resolved, err := s.resolveWorkloadHost(ctx, deployment.ServerID)
		if err != nil {
			// Falling back to localhost here would probe the control plane's
			// own machine and report the result as the workload's health.
			// An unresolvable target is an unknown, and unknown is not healthy.
			return &HealthCheckResult{
				Passed: false,
				Error:  fmt.Sprintf("health gate target unresolved for server %s: %v", deployment.ServerID, err),
			}, nil
		}
		host = resolved
	}
	target := fmt.Sprintf("http://%s:%d%s", host, deployment.HealthCheckPort, deployment.HealthCheckPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return &HealthCheckResult{Passed: false, Error: err.Error()}, nil
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthCheckResult{Passed: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	passed := resp.StatusCode >= 200 && resp.StatusCode < 400

	return &HealthCheckResult{
		Passed: passed,
		Status: resp.StatusCode,
		Body:   string(body),
	}, nil
}

// resolveWorkloadHost derives the address to probe from the node the workload
// actually runs on. The deployment's own HealthCheckHost wins when set; this
// is the fallback for deployments that only specify a path and port.
func (s *Service) resolveWorkloadHost(ctx context.Context, serverID string) (string, error) {
	if s == nil || s.store == nil {
		return "", errors.New("no store available to resolve the workload's node")
	}
	if serverID == "" {
		return "", errors.New("deployment has no server")
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return "", fmt.Errorf("resolve node for server: %w", err)
	}
	if target.NodeURL == "" {
		return "", errors.New("node has no base URL")
	}
	parsed, err := url.Parse(target.NodeURL)
	if err != nil {
		return "", fmt.Errorf("parse node base URL: %w", err)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("node base URL %q has no host", target.NodeURL)
	}
	return hostname, nil
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
			// ctx is already done here, so the audit write uses a detached
			// context: it must not inherit a deadline that has fired.
			_ = s.markStepFailed(context.WithoutCancel(ctx), stepID, fmt.Sprintf("health gate timed out after %d consecutive failures", threshold))
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
