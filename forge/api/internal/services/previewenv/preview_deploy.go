package previewenv

// Preview provisioning and teardown.
//
// A preview runs as an ordinary compose stack on a Beacon node: the ephemeral
// part is only who owns the row, what hostname it answers to and when it dies.
// Reusing the compose lifecycle (the same DeployComposeStack /
// DeleteComposeStack pair the Compose Stacks page drives) means previews get
// real placement, reservations, health gating and rollback instead of a
// second, weaker deployment path.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	composesvc "gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/trafficmanager"
)

// defaultProvisionTimeout bounds a single preview provisioning attempt. Compose
// deployment already waits for health, so this has to accommodate a slow image
// pull without letting a wedged node pin a goroutine forever.
const defaultProvisionTimeout = 15 * time.Minute

// PreviewDeployRequest is the deployment intent handed to the compose (or
// equivalent) runtime.
type PreviewDeployRequest struct {
	Name          string
	NodeID        string
	UserID        string
	ComposeYAML   string
	EnvVars       map[string]string
	MemoryMB      int64
	CPUShares     int64
	DiskMB        int64
	EnvironmentID string
}

// PreviewDeployResult reports the stack that now runs the preview.
type PreviewDeployResult struct {
	StackID string
	Status  string
}

// PreviewDeployer runs the real workload behind a preview. A deployer that
// cannot do the work must return an error; reporting success without a stack
// id is rejected in ProvisionPreview rather than passed through.
type PreviewDeployer interface {
	DeployPreview(ctx context.Context, req PreviewDeployRequest) (PreviewDeployResult, error)
	UpdatePreview(ctx context.Context, stackID string, req PreviewDeployRequest) (PreviewDeployResult, error)
	DestroyPreview(ctx context.Context, stackID string) error
}

// composePreviewDeployer adapts the compose lifecycle service to
// PreviewDeployer. It mirrors composeStackDeployer in cmd/api/main.go so both
// callers observe identical semantics.
type composePreviewDeployer struct {
	lifecycle *composesvc.Service
}

func (d composePreviewDeployer) DeployPreview(ctx context.Context, req PreviewDeployRequest) (PreviewDeployResult, error) {
	if d.lifecycle == nil {
		return PreviewDeployResult{}, errors.New("compose lifecycle service is not configured")
	}
	stack, err := d.lifecycle.DeployComposeStack(ctx, composesvc.DeployComposeRequest{
		UserID:        req.UserID,
		Name:          req.Name,
		NodeID:        req.NodeID,
		ComposeYAML:   req.ComposeYAML,
		EnvVars:       req.EnvVars,
		MemoryMB:      req.MemoryMB,
		CPUShares:     req.CPUShares,
		DiskMB:        req.DiskMB,
		EnvironmentID: req.EnvironmentID,
	})
	if err != nil {
		return PreviewDeployResult{}, err
	}
	if stack == nil || stack.ID == "" {
		return PreviewDeployResult{}, errors.New("compose deploy reported success without a stack")
	}
	return PreviewDeployResult{StackID: stack.ID, Status: string(stack.Status)}, nil
}

func (d composePreviewDeployer) UpdatePreview(ctx context.Context, stackID string, req PreviewDeployRequest) (PreviewDeployResult, error) {
	if d.lifecycle == nil {
		return PreviewDeployResult{}, errors.New("compose lifecycle service is not configured")
	}
	if stackID == "" {
		return d.DeployPreview(ctx, req)
	}
	stack, err := d.lifecycle.UpdateComposeStack(ctx, stackID, composesvc.UpdateComposeRequest{
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
		MemoryMB:    req.MemoryMB,
		CPUShares:   req.CPUShares,
		DiskMB:      req.DiskMB,
	})
	if err != nil {
		// A stack that no longer exists is not an update failure, it is a
		// fresh deploy; anything else must surface.
		if isPreviewStackGone(err) {
			return d.DeployPreview(ctx, req)
		}
		return PreviewDeployResult{}, err
	}
	if stack == nil || stack.ID == "" {
		return PreviewDeployResult{}, errors.New("compose update reported success without a stack")
	}
	return PreviewDeployResult{StackID: stack.ID, Status: string(stack.Status)}, nil
}

func (d composePreviewDeployer) DestroyPreview(ctx context.Context, stackID string) error {
	if d.lifecycle == nil {
		return errors.New("compose lifecycle service is not configured")
	}
	return d.lifecycle.DeleteComposeStack(ctx, stackID)
}

func isPreviewStackGone(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "not found") || strings.Contains(low, "no such stack")
}

// ---- provisioning ----

// startProvision runs the actual deployment in the background and returns
// immediately. The row it operates on is already persisted as "pending", so the
// caller observes every subsequent transition through the normal read
// endpoints; nothing is reported as finished here.
func (s *Service) startProvision(id, nodeID, userID string) {
	if s == nil || s.deployer == nil {
		return
	}
	parent := s.opts.BackgroundContext
	if parent == nil {
		parent = context.Background()
	}
	go s.provisionSafely(context.WithoutCancel(parent), id, nodeID, userID)
}

func (s *Service) provisionSafely(parent context.Context, id, nodeID, userID string) {
	defer func() {
		if rec := recover(); rec != nil && s.opts.Logger != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			s.opts.Logger.Error("previewenv: preview provisioning panic recovered",
				slog.String("preview", id), slog.String("panic", fmt.Sprint(rec)), slog.String("stack", string(buf[:n])))
		}
	}()
	ctx, cancel := context.WithTimeout(parent, defaultProvisionTimeout)
	defer cancel()
	if err := s.ProvisionPreview(ctx, id, nodeID, userID); err != nil && s.opts.Logger != nil {
		s.opts.Logger.Warn("previewenv: preview provisioning reported failure",
			slog.String("preview", id), slog.String("err", err.Error()))
	}
}

// ProvisionPreview moves a pending preview to "active" or "failed". It is
// exported so a caller that must know the outcome synchronously (tests, or a
// redeploy that wants to block) can call it directly instead of going through
// startProvision.
func (s *Service) ProvisionPreview(ctx context.Context, id, nodeID, userID string) error {
	repo, err := s.previews()
	if err != nil {
		return err
	}
	preview, err := repo.GetPreviewEnv(ctx, id)
	if err != nil {
		return err
	}
	if preview.Status != PreviewStatusPending && preview.Status != PreviewStatusDeploying {
		return fmt.Errorf("preview %s is %s and cannot be provisioned", preview.ID, preview.Status)
	}

	preview.Status = PreviewStatusDeploying
	preview.Error = ""
	preview.UpdatedAt = time.Now().UTC()
	if err := repo.UpdatePreviewEnv(ctx, preview); err != nil {
		return fmt.Errorf("mark preview deploying: %w", err)
	}
	s.publish(ctx, "preview_environment_deploying", preview.ID, map[string]any{
		"previewId": preview.ID,
		"branch":    preview.Branch,
	})

	// TLS/DNS for the wildcard is prepared before the stack starts so the
	// first request to the new hostname has a chance of terminating. Every
	// step is gated on an optional service and never blocks the preview.
	s.preparePreviewHost(ctx, preview)

	result, err := s.deployer.UpdatePreview(ctx, preview.StackID, PreviewDeployRequest{
		Name:          "preview-" + preview.Slug,
		NodeID:        nodeID,
		UserID:        userID,
		ComposeYAML:   preview.ComposeContent,
		EnvVars:       previewEnvVars(preview),
		MemoryMB:      s.opts.PreviewMemoryMB,
		CPUShares:     s.opts.PreviewCPUShares,
		DiskMB:        s.opts.PreviewDiskMB,
		EnvironmentID: derefOrEmpty(preview.EnvironmentID),
	})
	if err != nil {
		return s.failPreview(ctx, repo, preview, err)
	}
	if result.StackID == "" {
		return s.failPreview(ctx, repo, preview, errors.New("deployer reported success without a stack id"))
	}

	preview.StackID = result.StackID
	preview.Status = PreviewStatusActive
	preview.UpdatedAt = time.Now().UTC()
	if s.opts.PreviewTTL > 0 && preview.ExpiresAt == nil {
		expiry := time.Now().UTC().Add(s.opts.PreviewTTL)
		preview.ExpiresAt = &expiry
	}
	if err := repo.UpdatePreviewEnv(ctx, preview); err != nil {
		return fmt.Errorf("record preview as active: %w", err)
	}

	s.publish(ctx, "preview_environment_active", preview.ID, map[string]any{
		"previewId": preview.ID,
		"branch":    preview.Branch,
		"url":       preview.URL,
		"stackId":   preview.StackID,
		"status":    result.Status,
	})
	return nil
}

func (s *Service) failPreview(ctx context.Context, repo PreviewEnvStore, preview *PreviewDeployment, cause error) error {
	preview.Status = PreviewStatusFailed
	preview.Error = cause.Error()
	preview.UpdatedAt = time.Now().UTC()
	if err := repo.UpdatePreviewEnv(ctx, preview); err != nil {
		return errors.Join(cause, fmt.Errorf("record preview failure: %w", err))
	}
	if s.opts.Logger != nil {
		s.opts.Logger.Warn("previewenv: preview provisioning failed",
			slog.String("preview", preview.ID), slog.String("branch", preview.Branch), slog.String("err", cause.Error()))
	}
	s.publish(ctx, "preview_environment_failed", preview.ID, map[string]any{
		"previewId": preview.ID,
		"branch":    preview.Branch,
		"error":     cause.Error(),
	})
	return cause
}

// destroyPreviewStack stops the workload behind a preview. A stack that is
// already gone is not an error; a destroy that genuinely failed is, because a
// preview whose containers still run must never be recorded as closed.
func (s *Service) destroyPreviewStack(ctx context.Context, preview *PreviewDeployment) error {
	if preview.StackID == "" {
		return nil
	}
	if s.deployer == nil {
		return ErrPreviewRuntimeUnavailable
	}
	destroyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := s.deployer.DestroyPreview(destroyCtx, preview.StackID); err != nil && !isPreviewStackGone(err) {
		return fmt.Errorf("destroy preview stack %s: %w", preview.StackID, err)
	}
	preview.StackID = ""
	return nil
}

// ---- host preparation ----

// preparePreviewHost makes the preview hostname reachable. Certificates,
// wildcard DNS tracking and gateway rules are all optional services, so each
// step is gated and logged rather than fatal — the same contract Deploy()
// applies to the legacy preview path.
func (s *Service) preparePreviewHost(ctx context.Context, preview *PreviewDeployment) {
	if s.opts.AcmeService != nil {
		// Certificates are issued for the preview domain, not the legacy
		// per-PR base domain: they can be configured independently.
		if err := s.ensureCertificateForDomain(ctx, s.PreviewBaseDomain()); err != nil {
			s.warnPreview("previewenv: acme certificate step skipped", preview, err)
		}
	}

	serverID := derefOrEmpty(preview.ServerID)
	if s.opts.DomainSvc != nil && serverID != "" {
		wildcard := "*." + s.PreviewBaseDomain()
		existing, err := s.opts.DomainSvc.FindDomainByHost(ctx, wildcard)
		if err != nil && s.opts.Logger != nil {
			s.opts.Logger.Warn("previewenv: wildcard domain lookup failed",
				slog.String("preview", preview.ID), slog.String("wildcard", wildcard), slog.String("err", err.Error()))
		} else if err == nil && existing == nil {
			if _, err := s.opts.DomainSvc.AddDomain(ctx, serverID, wildcard); err != nil {
				s.warnPreview("previewenv: wildcard domain step skipped", preview, err)
			}
		}
	}

	if s.opts.TrafficMgr == nil || serverID == "" {
		return
	}
	host := strings.TrimPrefix(preview.URL, "https://")
	err := s.opts.TrafficMgr.CreateRoutingRule(ctx, &trafficmanager.RoutingRule{
		ID:         previewRouteID(preview.ID),
		Name:       previewRouteID(preview.ID),
		ServerID:   serverID,
		Domain:     host,
		TargetHost: "localhost",
		TargetPort: 8080,
		Protocol:   "https",
		Strategy:   "round_robin",
		Weight:     1,
		Enabled:    true,
		WebSocket:  true,
	})
	if err != nil {
		s.warnPreview("previewenv: traffic route step skipped", preview, err)
	}
}

// withdrawPreviewRoute removes the gateway rule preparePreviewHost created. It
// is a no-op when no traffic manager is configured, because then none was ever
// created.
func (s *Service) withdrawPreviewRoute(ctx context.Context, preview *PreviewDeployment) {
	if s == nil || s.opts.TrafficMgr == nil || preview == nil {
		return
	}
	if err := s.opts.TrafficMgr.DeleteRoutingRule(ctx, previewRouteID(preview.ID)); err != nil && s.opts.Logger != nil {
		s.opts.Logger.Warn("previewenv: preview traffic route not withdrawn",
			slog.String("preview", preview.ID), slog.String("err", err.Error()))
	}
}

func previewRouteID(id string) string { return "preview-env-" + id }

func (s *Service) warnPreview(msg string, preview *PreviewDeployment, err error) {
	if s.opts.Logger == nil || err == nil {
		return
	}
	s.opts.Logger.Warn(msg, slog.String("preview", preview.ID), slog.String("url", preview.URL), slog.String("err", err.Error()))
}

// ---- reaper ----

// StartPreviewReaper reaps TTL-expired project previews on a fixed interval. It
// is separate from the legacy reaper because the two tables have different
// expiry semantics ("expired, stack destroyed" vs. "cleaned_up, then row
// deleted").
func (s *Service) StartPreviewReaper(ctx context.Context, interval time.Duration) *previewReaper {
	if s == nil || s.previewsStore == nil {
		return nil
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	r := &previewReaper{svc: s, done: make(chan struct{})}
	go r.loop(ctx, interval)
	return r
}

type previewReaper struct {
	svc  *Service
	done chan struct{}
}

func (r *previewReaper) loop(ctx context.Context, interval time.Duration) {
	defer func() {
		if rec := recover(); rec != nil && r.svc.opts.Logger != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			r.svc.opts.Logger.Error("previewenv: preview reaper panic recovered",
				slog.String("panic", fmt.Sprint(rec)), slog.String("stack", string(buf[:n])))
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			close(r.done)
			return
		case <-ticker.C:
			reaped, err := r.svc.CleanupExpired(ctx)
			if r.svc.opts.Logger == nil {
				continue
			}
			if err != nil {
				r.svc.opts.Logger.Warn("previewenv: preview reaper pass incomplete",
					slog.Int("reaped", reaped), slog.String("err", err.Error()))
				continue
			}
			if reaped > 0 {
				r.svc.opts.Logger.Info("previewenv: preview reaper pass", slog.Int("reaped", reaped))
			}
		}
	}
}

// Stop waits for a running reaper pass to finish.
func (r *previewReaper) Stop() {
	if r == nil {
		return
	}
	<-r.done
}

// ---- helpers ----

// previewEnvVars stamps the preview's identity into the stack so the workload
// (and any label-driven ingress) can advertise the hostname it is being
// served on instead of guessing.
func previewEnvVars(p *PreviewDeployment) map[string]string {
	host := strings.TrimPrefix(p.URL, "https://")
	return map[string]string{
		"FORGE_PREVIEW":        "true",
		"FORGE_PREVIEW_ID":     p.ID,
		"FORGE_PREVIEW_BRANCH": p.Branch,
		"FORGE_PREVIEW_COMMIT": p.CommitSHA,
		"FORGE_PREVIEW_URL":    p.URL,
		"VIRTUAL_HOST":         host,
	}
}

func derefOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
