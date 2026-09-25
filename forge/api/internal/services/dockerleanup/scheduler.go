package dockerleanup

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// schedulerTickInterval is how often the leader renews its lease and scans
	// for due policies. 15s keeps worst-case dispatch lateness well under a
	// minute (the smallest cron granularity) without hammering the DB.
	schedulerTickInterval = 15 * time.Second
	// leaseTTL bounds how long a wedged leader can block failover. The lease is
	// renewed every tick, so healthy leaders never lose it.
	leaseTTL = 45 * time.Second
	// maxNodeConcurrency caps in-flight Beacon round-trips per policy run so a
	// large global fan-out cannot swamp the API or the node fleet.
	maxNodeConcurrency = 4
	// dueBatchSize limits one scan; leftovers are picked up next tick.
	dueBatchSize = 64
	// nextRunBackoff keeps a policy whose stored schedule became unparseable
	// from hot-looping the due scan.
	nextRunBackoff = 5 * time.Minute
	// policyRunTimeout bounds a single scheduled cleanup execution (disk-usage
	// query + image prune + optional cache/volume prune across all nodes).
	policyRunTimeout = 10 * time.Minute
	// leaseName is the singleton key in docker_cleanup_leader.
	leaseName = "docker-cleanup-scheduler"
)

// Start launches the background scheduler loop. Only the replica holding the
// store-based leader lease does work; followers wake every tick to try to take
// over, so a crashed leader is replaced within leaseTTL. The loop exits with
// ctx and releases the lease for a fast graceful handover.
func (s *Service) Start(ctx context.Context) error {
	if s.store == nil {
		return errors.New("dockerleanup: store required")
	}
	go s.loop(ctx)
	return nil
}

func (s *Service) loop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			s.logger.Error("docker cleanup scheduler panic recovered", "panic", r, "stack", string(buf[:n]))
		}
	}()

	leader := false
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	s.tickPhase(ctx, &leader)
	for {
		select {
		case <-ctx.Done():
			s.releaseLease()
			return
		case <-ticker.C:
			s.tickPhase(ctx, &leader)
		}
	}
}

// tickPhase renews/acquires leadership and, when in charge, runs one scan.
func (s *Service) tickPhase(ctx context.Context, leader *bool) {
	if ctx.Err() != nil {
		return
	}
	tickCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	held, err := s.tryAcquireLease(tickCtx)
	if err != nil {
		s.logger.Error("docker cleanup lease attempt failed", "error", err)
		return
	}
	if !held {
		if *leader {
			s.logger.Info("docker cleanup leadership moved to another instance", "instance", s.instanceID)
		}
		*leader = false
		return
	}
	wasLeader := *leader
	*leader = true

	if !wasLeader {
		s.logger.Info("acquired docker cleanup scheduler leadership", "instance", s.instanceID)
		s.backfillNextRuns(tickCtx)
	}
	s.runDuePolicies(tickCtx)
}

// backfillNextRuns computes the first occurrence for enabled policies whose
// next_run_at is null. Idempotent; safe to run on every leadership change.
func (s *Service) backfillNextRuns(ctx context.Context) {
	policies, err := s.listEnabledWithoutNextRun(ctx)
	if err != nil {
		s.logger.Error("docker cleanup next-run backfill scan failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for i := range policies {
		p := policies[i]
		next, err := s.nextRun(p.Schedule, now)
		if err != nil {
			s.logger.Error("skipping unparseable schedule during backfill", "policy_id", p.ID, "schedule", p.Schedule, "error", err)
			backoff := now.Add(nextRunBackoff)
			_ = s.updatePolicyRun(ctx, p.ID, nil, &backoff, nil, nil)
			continue
		}
		if err := s.updatePolicyRun(ctx, p.ID, nil, &next, nil, nil); err != nil {
			s.logger.Error("docker cleanup backfill update failed", "policy_id", p.ID, "error", err)
		}
	}
}

// runDuePolicies scans for policies whose occurrence came due and runs them.
// next_run_at is rolled forward BEFORE execution, so each occurrence is claimed
// exactly once by this leader even if a prune is slow or fails.
func (s *Service) runDuePolicies(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.listDuePolicies(ctx, now, dueBatchSize)
	if err != nil {
		s.logger.Error("docker cleanup due scan failed", "error", err)
		return
	}
	for i := range due {
		p := due[i]
		if ctx.Err() != nil {
			break
		}
		next, err := s.nextRun(p.Schedule, now)
		if err != nil {
			s.logger.Error("due policy has unparseable schedule; backing off", "policy_id", p.ID, "schedule", p.Schedule, "error", err)
			backoff := now.Add(nextRunBackoff)
			_ = s.updatePolicyRun(ctx, p.ID, nil, &backoff, nil, nil)
			continue
		}
		lastRun := now
		status := "running"
		// Claim the occurrence and stamp status atomically before dispatch.
		if err := s.updatePolicyRun(ctx, p.ID, &lastRun, &next, &status, nil); err != nil {
			s.logger.Error("failed to claim due docker cleanup policy", "policy_id", p.ID, "error", err)
			continue
		}

		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), policyRunTimeout)
		runErr := s.runPolicy(runCtx, p)
		cancel()

		finalStatus := "success"
		var lastErr *string
		if runErr != nil {
			finalStatus = "failed"
			msg := truncate(runErr.Error(), 2000)
			lastErr = &msg
			s.logger.Error("docker cleanup policy run failed", "policy_id", p.ID, "error", runErr)
		}
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err := s.updatePolicyRun(finishCtx, p.ID, nil, nil, &finalStatus, lastErr); err != nil {
			s.logger.Error("failed to record docker cleanup policy outcome", "policy_id", p.ID, "error", err)
		}
		finishCancel()
	}
}

// RunPolicyNow executes a policy immediately (manual trigger). It records the
// outcome but does NOT shift next_run_at — a manual run is an extra execution.
func (s *Service) RunPolicyNow(ctx context.Context, id string) (*CleanupPolicy, error) {
	p, err := s.GetPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, policyRunTimeout)
	runErr := s.runPolicy(runCtx, *p)
	cancel()

	status := "success"
	var lastErr *string
	runAt := time.Now().UTC()
	if runErr != nil {
		status = "failed"
		msg := truncate(runErr.Error(), 2000)
		lastErr = &msg
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	if err := s.updatePolicyRun(finishCtx, p.ID, &runAt, nil, &status, lastErr); err != nil {
		s.logger.Error("failed to record manual docker cleanup run", "policy_id", p.ID, "error", err)
	}
	updated, err := s.GetPolicy(ctx, id)
	if err != nil {
		return p, nil
	}
	if runErr != nil {
		return updated, runErr
	}
	return updated, nil
}

// runPolicy evaluates and prunes across the policy's target nodes. A global
// policy (empty NodeID) fans out to every reachable node; a host-scoped policy
// runs on just its node. Per-node failures are collected and surfaced as a
// joined error so one unreachable node does not abort the whole fleet sweep.
func (s *Service) runPolicy(ctx context.Context, p CleanupPolicy) error {
	targets, err := s.policyTargets(ctx, p)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	var (
		mu   sync.Mutex
		errs []string
		wg   sync.WaitGroup
		sem  = make(chan struct{}, maxNodeConcurrency)
	)
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(target NodeTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					s.logger.Error("docker cleanup node run panic recovered", "node", target.ID, "panic", r, "stack", string(buf[:n]))
					mu.Lock()
					errs = append(errs, fmt.Sprintf("node %s: panic", target.ID))
					mu.Unlock()
				}
			}()
			if err := s.cleanupNode(ctx, target, p.MostRecentLimit, p.PruneBuildCache, p.PruneVolumes); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("node %s: %v", target.ID, err))
				mu.Unlock()
			}
		}(target)
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", ErrDispatch, strings.Join(errs, "; "))
	}
	return nil
}

// cleanupNode runs one node's cleanup: prune the unused images (honouring the
// retention floor), then optionally the build cache and dangling volumes.
func (s *Service) cleanupNode(ctx context.Context, target NodeTarget, mostRecentLimit int, pruneBuildCache, pruneVolumes bool) error {
	usage, err := s.beaconDiskUsage(ctx, target)
	if err != nil {
		return err
	}
	unused := selectUnusedImages(usage.Images, mostRecentLimit)
	if len(unused) > 0 {
		ids := make([]string, 0, len(unused))
		for _, img := range unused {
			ids = append(ids, img.ID)
		}
		if _, err := s.beaconPrune(ctx, "POST", "/api/admin/docker-cleanup/prune-images", target, map[string]any{"imageIds": ids}); err != nil {
			return err
		}
	}
	if pruneBuildCache {
		if _, err := s.beaconPrune(ctx, "POST", "/api/admin/docker-cleanup/prune-build-cache", target, nil); err != nil {
			return err
		}
	}
	if pruneVolumes {
		if _, err := s.beaconPrune(ctx, "POST", "/api/admin/docker-cleanup/prune-volumes", target, nil); err != nil {
			return err
		}
	}
	return nil
}

// policyTargets resolves the concrete nodes a policy should run against.
func (s *Service) policyTargets(ctx context.Context, p CleanupPolicy) ([]NodeTarget, error) {
	if s.nodes == nil {
		return nil, fmt.Errorf("%w: node resolver unavailable", ErrDispatch)
	}
	if strings.TrimSpace(p.NodeID) == "" {
		return s.nodes.All(ctx)
	}
	target, err := s.nodes.Resolve(ctx, p.NodeID)
	if err != nil {
		return nil, fmt.Errorf("resolve node %s: %w", p.NodeID, err)
	}
	if strings.TrimSpace(target.URL) == "" {
		return nil, nil
	}
	return []NodeTarget{target}, nil
}

// ---- store queries (raw SQL over the shared pool; mirrors scheduledtasks) ----

func (s *Service) tryAcquireLease(ctx context.Context) (bool, error) {
	var holder string
	err := s.store.DB().QueryRow(ctx, `
		INSERT INTO docker_cleanup_leader (name, instance_id, acquired_at, expires_at)
		VALUES ($1, $2, NOW(), NOW() + make_interval(secs => $3))
		ON CONFLICT (name) DO UPDATE
		SET instance_id = EXCLUDED.instance_id, acquired_at = NOW(), expires_at = EXCLUDED.expires_at
		WHERE docker_cleanup_leader.expires_at < NOW() OR docker_cleanup_leader.instance_id = EXCLUDED.instance_id
		RETURNING instance_id`, leaseName, s.instanceID, leaseTTL.Seconds()).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return holder == s.instanceID, nil
}

func (s *Service) releaseLease() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.store.DB().Exec(ctx, `
		DELETE FROM docker_cleanup_leader WHERE name = $1 AND instance_id = $2`, leaseName, s.instanceID)
	if err != nil {
		s.logger.Warn("failed to release docker cleanup lease on shutdown", "instance", s.instanceID, "error", err)
	}
}

func (s *Service) listEnabledWithoutNextRun(ctx context.Context) ([]CleanupPolicy, error) {
	rows, err := s.store.DB().Query(ctx, `SELECT `+policyColumns+` FROM docker_cleanup_policies
		WHERE enabled AND next_run_at IS NULL ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPolicies(rows)
}

func (s *Service) listDuePolicies(ctx context.Context, now time.Time, limit int) ([]CleanupPolicy, error) {
	if limit <= 0 || limit > 256 {
		limit = dueBatchSize
	}
	rows, err := s.store.DB().Query(ctx, `SELECT `+policyColumns+` FROM docker_cleanup_policies
		WHERE enabled AND next_run_at IS NOT NULL AND next_run_at <= $1
		ORDER BY next_run_at ASC LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectPolicies(rows)
}

func collectPolicies(rows pgx.Rows) ([]CleanupPolicy, error) {
	policies := []CleanupPolicy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// updatePolicyRun stamps run metadata with keep-existing semantics for null
// arguments (COALESCE), except last_error which is explicitly clearable: pass a
// pointer to "" to blank it. Any argument may be nil to leave that column alone.
func (s *Service) updatePolicyRun(ctx context.Context, id string, lastRun, nextRun *time.Time, status, lastError *string) error {
	_, err := s.store.DB().Exec(ctx, `
		UPDATE docker_cleanup_policies
		SET last_run_at = COALESCE($2, last_run_at),
		    next_run_at = COALESCE($3, next_run_at),
		    last_status = COALESCE($4, last_status),
		    last_error  = CASE WHEN $5::text IS NULL THEN last_error ELSE $5::text END,
		    updated_at  = now()
		WHERE id = $1`,
		id, lastRun, nextRun, status, lastError)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}
